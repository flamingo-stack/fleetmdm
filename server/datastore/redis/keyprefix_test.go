// OPENFRAME(redis-key-prefix): unit tests for the per-tenant key-prefix wrapper —
// openframe/docs/redis-key-prefix.md
//
// This is the highest-value guard for the redis-key-prefix feature: a silent
// regression here (e.g. an upstream refactor that bypasses prefixArgs) means one
// tenant's keys land in another tenant's namespace. The tests are pure logic and
// need no live Redis.
package redis

// >>> OPENFRAME(redis-key-prefix): openframe/docs/redis-key-prefix.md

import (
	"reflect"
	"testing"

	redigo "github.com/gomodule/redigo/redis"
)

func TestPrefixArgs(t *testing.T) {
	const p = "t:" // already-normalized prefix

	cases := []struct {
		name string
		cmd  string
		args []interface{}
		want []interface{}
	}{
		// default rule: arg[0] is the key
		{"GET", "GET", []interface{}{"k"}, []interface{}{"t:k"}},
		{"SET key+value", "SET", []interface{}{"k", "v"}, []interface{}{"t:k", "v"}},
		{"HSET", "HSET", []interface{}{"h", "f", "v"}, []interface{}{"t:h", "f", "v"}},
		{"EXPIRE", "EXPIRE", []interface{}{"k", 30}, []interface{}{"t:k", 30}},
		{"PUBLISH channel at arg0", "PUBLISH", []interface{}{"ch", "msg"}, []interface{}{"t:ch", "msg"}},
		{"lowercase command", "get", []interface{}{"k"}, []interface{}{"t:k"}},
		{"unknown future command defaults to arg0", "FUTURECMD", []interface{}{"k", "x"}, []interface{}{"t:k", "x"}},

		// noPrefixCmds: nothing is a key
		{"PING", "PING", nil, nil},
		{"INFO", "INFO", []interface{}{"server"}, []interface{}{"server"}},
		{"MULTI", "MULTI", nil, nil},
		{"EXEC", "EXEC", nil, nil},
		{"SELECT", "SELECT", []interface{}{0}, []interface{}{0}},
		{"CLUSTER subcommand", "CLUSTER", []interface{}{"INFO"}, []interface{}{"INFO"}},
		{"SCRIPT subcommand", "SCRIPT", []interface{}{"LOAD", "return 1"}, []interface{}{"LOAD", "return 1"}},

		// allArgs: every arg is a key/channel
		{"DEL", "DEL", []interface{}{"a", "b", "c"}, []interface{}{"t:a", "t:b", "t:c"}},
		{"MGET", "MGET", []interface{}{"a", "b"}, []interface{}{"t:a", "t:b"}},
		{"EXISTS", "EXISTS", []interface{}{"a", "b"}, []interface{}{"t:a", "t:b"}},
		{"SUBSCRIBE channels", "SUBSCRIBE", []interface{}{"c1", "c2"}, []interface{}{"t:c1", "t:c2"}},
		{"PSUBSCRIBE channels", "PSUBSCRIBE", []interface{}{"c1"}, []interface{}{"t:c1"}},

		// twoKeys: arg[0] and arg[1]
		{"RENAME", "RENAME", []interface{}{"a", "b"}, []interface{}{"t:a", "t:b"}},
		{"SMOVE keeps member", "SMOVE", []interface{}{"src", "dst", "member"}, []interface{}{"t:src", "t:dst", "member"}},
		{"COPY", "COPY", []interface{}{"a", "b"}, []interface{}{"t:a", "t:b"}},

		// eval: arg[1] is numKeys
		{"EVAL 2 keys", "EVAL", []interface{}{"script", 2, "k1", "k2", "argv1"}, []interface{}{"script", 2, "t:k1", "t:k2", "argv1"}},
		{"EVALSHA 1 key", "EVALSHA", []interface{}{"sha", 1, "k", "argv"}, []interface{}{"sha", 1, "t:k", "argv"}},
		{"EVAL 0 keys prefixes nothing", "EVAL", []interface{}{"script", 0, "argv"}, []interface{}{"script", 0, "argv"}},

		// scan: only the MATCH pattern
		{"SCAN with MATCH", "SCAN", []interface{}{"0", "MATCH", "pat*", "COUNT", 10}, []interface{}{"0", "MATCH", "t:pat*", "COUNT", 10}},
		{"SCAN without MATCH leaves args (documented caveat)", "SCAN", []interface{}{"0", "COUNT", 10}, []interface{}{"0", "COUNT", 10}},

		// bitop: op at arg[0], keys at arg[1..]
		{"BITOP", "BITOP", []interface{}{"AND", "dst", "s1", "s2"}, []interface{}{"AND", "t:dst", "t:s1", "t:s2"}},

		// object: key at arg[1] for the inspecting subcommands
		{"OBJECT ENCODING", "OBJECT", []interface{}{"ENCODING", "k"}, []interface{}{"ENCODING", "t:k"}},

		// blocking pop: all keys before the trailing timeout
		{"BLPOP", "BLPOP", []interface{}{"k1", "k2", 5}, []interface{}{"t:k1", "t:k2", 5}},

		// sort: source key + STORE destination
		{"SORT with STORE", "SORT", []interface{}{"mylist", "STORE", "dest"}, []interface{}{"t:mylist", "STORE", "t:dest"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := prefixArgs(tc.cmd, tc.args, p)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("prefixArgs(%q, %v) = %v, want %v", tc.cmd, tc.args, got, tc.want)
			}
		})
	}
}

func TestPrefixArgs_EmptyPrefixIsNoop(t *testing.T) {
	args := []interface{}{"k", "v"}
	got := prefixArgs("SET", args, "")
	if !reflect.DeepEqual(got, args) {
		t.Errorf("empty prefix should not modify args: got %v", got)
	}
}

func TestPrefixArgs_ByteKeys(t *testing.T) {
	got := prefixArgs("GET", []interface{}{[]byte("k")}, "t:")
	want := []interface{}{"t:k"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("[]byte key: got %v, want %v", got, want)
	}
}

func TestNormalizeKeyPrefix(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"tenant1", "tenant1:", false},
		{"tenant1:", "tenant1:", false},
		{"a{b", "", true},
		{"a}b", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeKeyPrefix(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("normalizeKeyPrefix(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("normalizeKeyPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// fakeConn is a minimal redigo.Conn for testing the wrapper plumbing.
type fakeConn struct{}

func (fakeConn) Close() error                                   { return nil }
func (fakeConn) Err() error                                     { return nil }
func (fakeConn) Do(string, ...interface{}) (interface{}, error) { return nil, nil }
func (fakeConn) Send(string, ...interface{}) error              { return nil }
func (fakeConn) Flush() error                                   { return nil }
func (fakeConn) Receive() (interface{}, error)                  { return nil, nil }

func TestNewPrefixedConn_Guards(t *testing.T) {
	var fc redigo.Conn = fakeConn{}

	if got := newPrefixedConn(nil, "t:"); got != nil {
		t.Errorf("nil inner should return nil, got %v", got)
	}
	if got := newPrefixedConn(fc, ""); got != fc {
		t.Errorf("empty prefix should return the inner conn unwrapped")
	}

	wrapped := newPrefixedConn(fc, "t:")
	if _, ok := wrapped.(*prefixedConn); !ok {
		t.Fatalf("expected *prefixedConn, got %T", wrapped)
	}
	// double-wrap guard: wrapping an already-wrapped conn must not nest.
	again := newPrefixedConn(wrapped, "t:")
	inner, prefix := unwrapConn(again)
	if prefix != "t:" {
		t.Errorf("unwrapConn prefix = %q, want %q", prefix, "t:")
	}
	if _, stillWrapped := inner.(*prefixedConn); stillWrapped {
		t.Errorf("double-wrap guard failed: inner is still a *prefixedConn")
	}
}

func TestUnwrapConn_Plain(t *testing.T) {
	var fc redigo.Conn = fakeConn{}
	got, prefix := unwrapConn(fc)
	if got != fc || prefix != "" {
		t.Errorf("unwrapConn(plain) = (%v, %q), want (fc, \"\")", got, prefix)
	}
}

// <<< OPENFRAME(redis-key-prefix)
