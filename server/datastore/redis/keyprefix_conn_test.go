// OPENFRAME(redis-key-prefix): unit tests for the prefixedConn wrapper plumbing
// and the prefix rules not covered by keyprefix_test.go —
// openframe/docs/redis-key-prefix.md
//
// These guard the paths where a regression is silent: a Do/Send that stops
// routing through prefixArgs, a PUBSUB introspection that leaks other tenants'
// channels, or a Bind that registers unprefixed keys with redisc.
package redis

import (
	"reflect"
	"testing"
	"time"

	redigo "github.com/gomodule/redigo/redis"
)

// recordingConn captures the command and args the wrapper forwards to the
// inner conn. It also implements ConnWithTimeout, Bind, and ReadOnly so the
// wrapper's optional-interface forwarding is testable.
type recordingConn struct {
	cmds     []string
	args     [][]interface{}
	bound    []string
	readOnly bool
}

func (c *recordingConn) Close() error { return nil }
func (c *recordingConn) Err() error   { return nil }
func (c *recordingConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	c.cmds = append(c.cmds, cmd)
	c.args = append(c.args, args)
	return nil, nil
}
func (c *recordingConn) Send(cmd string, args ...interface{}) error {
	c.cmds = append(c.cmds, cmd)
	c.args = append(c.args, args)
	return nil
}
func (c *recordingConn) Flush() error                  { return nil }
func (c *recordingConn) Receive() (interface{}, error) { return nil, nil }
func (c *recordingConn) DoWithTimeout(_ time.Duration, cmd string, args ...interface{}) (interface{}, error) {
	return c.Do(cmd, args...)
}
func (c *recordingConn) ReceiveWithTimeout(time.Duration) (interface{}, error) { return nil, nil }
func (c *recordingConn) Bind(keys ...string) error {
	c.bound = append(c.bound, keys...)
	return nil
}
func (c *recordingConn) ReadOnly() error {
	c.readOnly = true
	return nil
}

func (c *recordingConn) lastArgs(t *testing.T) []interface{} {
	t.Helper()
	if len(c.args) == 0 {
		t.Fatal("inner conn received no command")
	}
	return c.args[len(c.args)-1]
}

func TestPrefixedConnDoPrefixesKeys(t *testing.T) {
	rc := &recordingConn{}
	pc := newPrefixedConn(rc, "t:")

	if _, err := pc.Do("SET", "k", "v"); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []interface{}{"t:k", "v"}; !reflect.DeepEqual(rc.lastArgs(t), want) {
		t.Errorf("Do forwarded %v, want %v", rc.lastArgs(t), want)
	}
}

func TestPrefixedConnSendPrefixesKeys(t *testing.T) {
	rc := &recordingConn{}
	pc := newPrefixedConn(rc, "t:")

	if err := pc.Send("DEL", "a", "b"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if want := []interface{}{"t:a", "t:b"}; !reflect.DeepEqual(rc.lastArgs(t), want) {
		t.Errorf("Send forwarded %v, want %v", rc.lastArgs(t), want)
	}
}

func TestPrefixedConnDoWithTimeoutPrefixesKeys(t *testing.T) {
	rc := &recordingConn{}
	pc := newPrefixedConn(rc, "t:").(*prefixedConn)

	if _, err := pc.DoWithTimeout(time.Second, "GET", "k"); err != nil {
		t.Fatalf("DoWithTimeout: %v", err)
	}
	if want := []interface{}{"t:k"}; !reflect.DeepEqual(rc.lastArgs(t), want) {
		t.Errorf("DoWithTimeout forwarded %v, want %v", rc.lastArgs(t), want)
	}
}

func TestPrefixedConnBind(t *testing.T) {
	rc := &recordingConn{}
	pc := newPrefixedConn(rc, "t:").(*prefixedConn)

	if err := pc.Bind("k1", "k2"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if want := []string{"t:k1", "t:k2"}; !reflect.DeepEqual(rc.bound, want) {
		t.Errorf("Bind forwarded %v, want %v", rc.bound, want)
	}
}

func TestPrefixedConnBindInnerWithoutBind(t *testing.T) {
	var fc redigo.Conn = fakeConn{} // fakeConn has no Bind
	pc := newPrefixedConn(fc, "t:").(*prefixedConn)
	if err := pc.Bind("k"); err == nil {
		t.Error("Bind on an inner conn without Bind must error, not panic or no-op")
	}
}

func TestPrefixedConnReadOnly(t *testing.T) {
	rc := &recordingConn{}
	pc := newPrefixedConn(rc, "t:").(*prefixedConn)

	if err := pc.ReadOnly(); err != nil {
		t.Fatalf("ReadOnly: %v", err)
	}
	if !rc.readOnly {
		t.Error("ReadOnly was not forwarded to the inner conn")
	}

	var fc redigo.Conn = fakeConn{}
	pcPlain := newPrefixedConn(fc, "t:").(*prefixedConn)
	if err := pcPlain.ReadOnly(); err == nil {
		t.Error("ReadOnly on an inner conn without ReadOnly must error")
	}
}

// TestPrefixArgs_MoreRules covers the rules and commands the main table in
// keyprefix_test.go does not: PUBSUB introspection, WATCH, store-variant
// multi-key commands, SORT without STORE, FCALL, and numKeys-as-string EVAL.
func TestPrefixArgs_MoreRules(t *testing.T) {
	const p = "t:"

	cases := []struct {
		name string
		cmd  string
		args []interface{}
		want []interface{}
	}{
		// pubsubArgs: only the channel-taking subcommands get prefixed
		{"PUBSUB CHANNELS", "PUBSUB", []interface{}{"CHANNELS", "ch*"}, []interface{}{"CHANNELS", "t:ch*"}},
		{"PUBSUB NUMSUB two channels", "PUBSUB", []interface{}{"NUMSUB", "c1", "c2"}, []interface{}{"NUMSUB", "t:c1", "t:c2"}},
		{"PUBSUB SHARDCHANNELS", "PUBSUB", []interface{}{"SHARDCHANNELS", "ch*"}, []interface{}{"SHARDCHANNELS", "t:ch*"}},
		{"PUBSUB SHARDNUMSUB", "PUBSUB", []interface{}{"SHARDNUMSUB", "c1"}, []interface{}{"SHARDNUMSUB", "t:c1"}},
		{"PUBSUB NUMPAT has no channels", "PUBSUB", []interface{}{"NUMPAT"}, []interface{}{"NUMPAT"}},
		{"PUBSUB with no subcommand", "PUBSUB", nil, []interface{}{}},

		// transactions: WATCH keys are prefixed, UNWATCH/MULTI/EXEC are not
		{"WATCH", "WATCH", []interface{}{"k1", "k2"}, []interface{}{"t:k1", "t:k2"}},

		// store variants where every arg is a key
		{"ZUNIONSTORE", "ZUNIONSTORE", []interface{}{"dst", "s1", "s2"}, []interface{}{"t:dst", "t:s1", "t:s2"}},
		{"PFMERGE", "PFMERGE", []interface{}{"dst", "s1"}, []interface{}{"t:dst", "t:s1"}},

		// twoKeys variants
		{"LMOVE keeps directions", "LMOVE", []interface{}{"src", "dst", "LEFT", "RIGHT"}, []interface{}{"t:src", "t:dst", "LEFT", "RIGHT"}},
		{"ZRANGESTORE keeps range", "ZRANGESTORE", []interface{}{"dst", "src", 0, -1}, []interface{}{"t:dst", "t:src", 0, -1}},
		{"BRPOPLPUSH keeps timeout", "BRPOPLPUSH", []interface{}{"src", "dst", 5}, []interface{}{"t:src", "t:dst", 5}},

		// blocking pops
		{"BZPOPMIN", "BZPOPMIN", []interface{}{"z1", "z2", 0}, []interface{}{"t:z1", "t:z2", 0}},
		{"BLPOP single key", "BLPOP", []interface{}{"k", 1}, []interface{}{"t:k", 1}},

		// sort without STORE: only the source key
		{"SORT plain", "SORT", []interface{}{"mylist", "LIMIT", 0, 10}, []interface{}{"t:mylist", "LIMIT", 0, 10}},

		// scripting: numKeys arrives as a string over the wire too
		{"EVAL numKeys as string", "EVAL", []interface{}{"script", "2", "k1", "k2", "argv"}, []interface{}{"script", "2", "t:k1", "t:k2", "argv"}},
		{"FCALL", "FCALL", []interface{}{"fn", 1, "k", "argv"}, []interface{}{"fn", 1, "t:k", "argv"}},
		{"EVAL with numKeys beyond args stays in bounds", "EVAL", []interface{}{"script", 5, "k1"}, []interface{}{"script", 5, "t:k1"}},
		{"EVAL single arg untouched", "EVAL", []interface{}{"script"}, []interface{}{"script"}},

		// object: non-inspecting subcommand untouched
		{"OBJECT HELP", "OBJECT", []interface{}{"HELP"}, []interface{}{"HELP"}},

		// shard pub/sub channels
		{"SSUBSCRIBE", "SSUBSCRIBE", []interface{}{"c1"}, []interface{}{"t:c1"}},

		// empty args on a default-rule command must not panic
		{"GET with no args", "GET", nil, []interface{}{}},
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

func TestPrefixArgs_DoesNotMutateInput(t *testing.T) {
	args := []interface{}{"k", "v"}
	_ = prefixArgs("SET", args, "t:")
	if args[0] != "k" {
		t.Errorf("prefixArgs mutated the caller's args slice: %v", args)
	}
}

func TestPrefixOne_OutOfRange(t *testing.T) {
	args := []interface{}{"k"}
	prefixOne(args, -1, "t:") // must not panic
	prefixOne(args, 1, "t:")  // must not panic
	if args[0] != "k" {
		t.Errorf("out-of-range prefixOne mutated args: %v", args)
	}
}

func TestToInt(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{2, 2},
		{int64(3), 3},
		{int32(4), 4},
		{uint(5), 5},
		{uint64(6), 6},
		{"7", 7},
		{"not a number", 0},
		{nil, 0},
		{3.9, 0}, // unsupported type: fail closed to 0 keys, not a partial prefix
	}
	for _, tc := range cases {
		if got := toInt(tc.in); got != tc.want {
			t.Errorf("toInt(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestToString(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{"s", "s"},
		{[]byte("b"), "b"},
		{42, "42"},
	}
	for _, tc := range cases {
		if got := toString(tc.in); got != tc.want {
			t.Errorf("toString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
