package redis

// OPENFRAME BEGIN: fork-specific multi-tenant key-prefixing wrapper.
// This entire file is fork-only logic added on top of the shared upstream
// fleetdm/fleet server/datastore/redis package. It implements Redis key/
// channel prefixing for multi-tenant isolation and does not exist upstream.
// On upstream syncs of this package, this file must be preserved as-is;
// nothing here should be merged/overwritten from upstream.

import (
	"fmt"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
)

// prefixedConn prepends keyPrefix to keys/channels on every Do/Send. Policy
// is fail-closed: default = prefix arg[0]; noPrefixCmds and specialCmds
// override. redisc.RetryConn rejects the wrapper, so unwrap before passing in.
type prefixedConn struct {
	inner  redis.Conn
	prefix string
}

func newPrefixedConn(inner redis.Conn, prefix string) redis.Conn {
	if prefix == "" || inner == nil {
		return inner
	}
	if _, already := inner.(*prefixedConn); already {
		return inner
	}
	return &prefixedConn{inner: inner, prefix: prefix}
}

func unwrapConn(c redis.Conn) (redis.Conn, string) {
	if pc, ok := c.(*prefixedConn); ok {
		return pc.inner, pc.prefix
	}
	return c, ""
}

func (p *prefixedConn) Close() error                  { return p.inner.Close() }
func (p *prefixedConn) Err() error                    { return p.inner.Err() }
func (p *prefixedConn) Flush() error                  { return p.inner.Flush() }
func (p *prefixedConn) Receive() (interface{}, error) { return p.inner.Receive() }

func (p *prefixedConn) Do(cmd string, args ...interface{}) (interface{}, error) {
	return p.inner.Do(cmd, prefixArgs(cmd, args, p.prefix)...)
}

func (p *prefixedConn) Send(cmd string, args ...interface{}) error {
	return p.inner.Send(cmd, prefixArgs(cmd, args, p.prefix)...)
}

// DoWithTimeout / ReceiveWithTimeout satisfy redigo.ConnWithTimeout so callers
// (e.g. PubSubConn.ReceiveWithTimeout) work through the wrapper.
func (p *prefixedConn) DoWithTimeout(timeout time.Duration, cmd string, args ...interface{}) (interface{}, error) {
	return redis.DoWithTimeout(p.inner, timeout, cmd, prefixArgs(cmd, args, p.prefix)...)
}

func (p *prefixedConn) ReceiveWithTimeout(timeout time.Duration) (interface{}, error) {
	return redis.ReceiveWithTimeout(p.inner, timeout)
}

// Bind prefixes the keys and forwards to inner conn so redisc.BindConn works.
func (p *prefixedConn) Bind(keys ...string) error {
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = p.prefix + k
	}
	cc, ok := p.inner.(interface {
		Bind(...string) error
	})
	if !ok {
		return fmt.Errorf("redis: inner conn does not implement Bind")
	}
	return cc.Bind(prefixed...)
}

// ReadOnly forwards to inner conn so redisc.ReadOnlyConn works.
func (p *prefixedConn) ReadOnly() error {
	cc, ok := p.inner.(interface {
		ReadOnly() error
	})
	if !ok {
		return fmt.Errorf("redis: inner conn does not implement ReadOnly")
	}
	return cc.ReadOnly()
}

// prefixArgs prepends keyPrefix to key/channel args per the policy above.
func prefixArgs(cmd string, args []interface{}, prefix string) []interface{} {
	if prefix == "" {
		return args
	}
	up := strings.ToUpper(cmd)
	if noPrefixCmds[up] {
		return args
	}
	out := make([]interface{}, len(args))
	copy(out, args)
	if rule, ok := specialCmds[up]; ok {
		rule(out, prefix)
		return out
	}
	// Default: arg[0] is the key — safe for unknown/future commands too.
	if len(out) > 0 {
		prefixOne(out, 0, prefix)
	}
	return out
}

func prefixOne(args []interface{}, idx int, prefix string) {
	if idx < 0 || idx >= len(args) {
		return
	}
	args[idx] = prefix + toString(args[idx])
}

func toString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

type prefixRule func(args []interface{}, prefix string)

// allArgs: every arg is a key/channel (DEL, MGET, SUBSCRIBE, ...).
func allArgs(args []interface{}, prefix string) {
	for i := range args {
		prefixOne(args, i, prefix)
	}
}

// twoKeys: args[0] and args[1] are both keys (RENAME, SMOVE, COPY, ...).
func twoKeys(args []interface{}, prefix string) {
	prefixOne(args, 0, prefix)
	prefixOne(args, 1, prefix)
}

// evalArgs: args = [script, numKeys, k1, ..., kN, arg1, ...].
func evalArgs(args []interface{}, prefix string) {
	if len(args) < 2 {
		return
	}
	n := toInt(args[1])
	if n <= 0 {
		return
	}
	for i := 0; i < n; i++ {
		prefixOne(args, 2+i, prefix)
	}
}

// scanArgs: prefix the MATCH pattern. Callers must pass MATCH or SCAN sees
// all tenants' keys (ScanKeys in redis.go always passes it).
func scanArgs(args []interface{}, prefix string) {
	for i := 0; i < len(args)-1; i++ {
		if strings.EqualFold(toString(args[i]), "MATCH") {
			prefixOne(args, i+1, prefix)
			return
		}
	}
}

// bitopArgs: BITOP op dst src... — args[0] is the op, args[1..] are keys.
func bitopArgs(args []interface{}, prefix string) {
	for i := 1; i < len(args); i++ {
		prefixOne(args, i, prefix)
	}
}

// objectArgs: OBJECT ENCODING/IDLETIME/FREQ/REFCOUNT key — key at args[1].
// For any other/unrecognized subcommand (e.g. HELP, or a future subcommand
// this list doesn't know about) we fail closed and prefix args[1] anyway if
// present, since silently forwarding an unprefixed potential key argument
// risks cross-tenant key access. HELP takes no key argument, so prefixing an
// absent/irrelevant arg[1] is a no-op in practice for that case.
func objectArgs(args []interface{}, prefix string) {
	if len(args) < 2 {
		return
	}
	sub := strings.ToUpper(toString(args[0]))
	switch sub {
	case "ENCODING", "IDLETIME", "FREQ", "REFCOUNT":
		prefixOne(args, 1, prefix)
	default:
		prefixOne(args, 1, prefix)
	}
}

// pubsubArgs: NUMSUB/CHANNELS/SHARD* take channel args (at args[1..]). For
// any other/unrecognized subcommand, fail closed by prefixing all remaining
// args as channels too, since forwarding them unprefixed risks cross-tenant
// channel leakage if a future subcommand also takes channel arguments.
func pubsubArgs(args []interface{}, prefix string) {
	if len(args) < 1 {
		return
	}
	sub := strings.ToUpper(toString(args[0]))
	switch sub {
	case "NUMSUB", "CHANNELS", "SHARDCHANNELS", "SHARDNUMSUB":
		for i := 1; i < len(args); i++ {
			prefixOne(args, i, prefix)
		}
	default:
		for i := 1; i < len(args); i++ {
			prefixOne(args, i, prefix)
		}
	}
}

// blockingKeysArgs: BLPOP/BRPOP/BZPOP* key... timeout — all but last are keys.
func blockingKeysArgs(args []interface{}, prefix string) {
	if len(args) < 2 {
		return
	}
	for i := 0; i < len(args)-1; i++ {
		prefixOne(args, i, prefix)
	}
}

// sortArgs: prefix the source key and, if present, the STORE destination.
// BY/GET patterns are left unprefixed — Fleet doesn't use them.
func sortArgs(args []interface{}, prefix string) {
	if len(args) == 0 {
		return
	}
	prefixOne(args, 0, prefix)
	for i := 1; i < len(args)-1; i++ {
		if strings.EqualFold(toString(args[i]), "STORE") {
			prefixOne(args, i+1, prefix)
			return
		}
	}
}

func toInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case int32:
		return int(x)
	case uint:
		return int(x)
	case uint64:
		return int(x)
	case string:
		var n int
		_, _ = fmt.Sscanf(x, "%d", &n)
		return n
	default:
		return 0
	}
}

// noPrefixCmds: commands with no key/channel args (admin/connection/info/
// transactions). Anything not here and not in specialCmds gets arg[0] prefixed.
var noPrefixCmds = map[string]bool{
	// connection / handshake
	"PING":      true,
	"AUTH":      true,
	"HELLO":     true,
	"SELECT":    true,
	"ECHO":      true,
	"QUIT":      true,
	"RESET":     true,
	"READONLY":  true,
	"READWRITE": true,

	// server / introspection
	"INFO":         true,
	"DBSIZE":       true,
	"TIME":         true,
	"ROLE":         true,
	"LASTSAVE":     true,
	"SHUTDOWN":     true,
	"BGSAVE":       true,
	"BGREWRITEAOF": true,
	"FLUSHDB":      true,
	"FLUSHALL":     true,
	"COMMAND":      true,
	"WAIT":         true,
	"FAILOVER":     true,
	"REPLICAOF":    true,
	"SLAVEOF":      true,

	// subcommand namespaces — none of their args are bare keys
	"CLIENT":   true,
	"CLUSTER":  true,
	"CONFIG":   true,
	"DEBUG":    true,
	"LATENCY":  true,
	"SLOWLOG":  true,
	"MEMORY":   true,
	"ACL":      true,
	"SCRIPT":   true,
	"FUNCTION": true,

	// transactions (WATCH is multi-key and handled in specialCmds)
	"MULTI":   true,
	"EXEC":    true,
	"DISCARD": true,
	"UNWATCH": true,
}

// specialCmds: commands with keys/channels NOT at arg[0]. Everything else
// falls through to the default "prefix arg[0]" rule.
var specialCmds = map[string]prefixRule{
	// every arg is a key
	"DEL":    allArgs,
	"UNLINK": allArgs,
	"MGET":   allArgs,
	"EXISTS": allArgs,
	"TOUCH":  allArgs,
	"WATCH":  allArgs,

	// dst + src(s) — every arg is a key
	"SINTERSTORE": allArgs,
	"SUNIONSTORE": allArgs,
	"SDIFFSTORE":  allArgs,
	"ZINTERSTORE": allArgs,
	"ZUNIONSTORE": allArgs,
	"ZDIFFSTORE":  allArgs,
	"PFCOUNT":     allArgs,
	"PFMERGE":     allArgs,

	// args[0] and args[1] are both keys
	"RENAME":         twoKeys,
	"RENAMENX":       twoKeys,
	"SMOVE":          twoKeys,
	"COPY":           twoKeys,
	"LMOVE":          twoKeys,
	"BLMOVE":         twoKeys,
	"RPOPLPUSH":      twoKeys,
	"BRPOPLPUSH":     twoKeys,
	"LCS":            twoKeys,
	"ZRANGESTORE":    twoKeys,
	"GEOSEARCHSTORE": twoKeys,

	// BITOP op dst src [src...] — args[1..] are keys
	"BITOP": bitopArgs,

	// OBJECT <sub> key — key at args[1] for ENCODING/IDLETIME/FREQ/REFCOUNT
	"OBJECT": objectArgs,

	// scripting — args[1] is numKeys
	"EVAL":     evalArgs,
	"EVALSHA":  evalArgs,
	"FCALL":    evalArgs,
	"FCALL_RO": evalArgs,

	// SCAN — MATCH pattern
	"SCAN": scanArgs,

	// SORT key ... [STORE dst]
	"SORT": sortArgs,

	// pub/sub channel commands — every arg is a channel
	"SUBSCRIBE":    allArgs,
	"UNSUBSCRIBE":  allArgs,
	"PSUBSCRIBE":   allArgs,
	"PUNSUBSCRIBE": allArgs,
	"SSUBSCRIBE":   allArgs,
	"SUNSUBSCRIBE": allArgs,
	"PUBSUB":       pubsubArgs,

	// blocking pop — BLPOP key [key ...] timeout
	"BLPOP":    blockingKeysArgs,
	"BRPOP":    blockingKeysArgs,
	"BZPOPMIN": blockingKeysArgs,
	"BZPOPMAX": blockingKeysArgs,
}

// OPENFRAME END: fork-specific multi-tenant key-prefixing wrapper.
