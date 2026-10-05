// >>> OPENFRAME(redis-seed-nodes): unit test for comma-separated cluster seed-node
// parsing — openframe/docs/redis-key-prefix.md
//
// This guards a regression that already bit once: the sync reset redis.go toward
// upstream and dropped splitSeedNodes, so a comma-separated FLEET_REDIS_ADDRESS
// was passed to the cluster as one node and startup failed with "too many colons
// in address". Pure logic, no live Redis.
package redis

import (
	"reflect"
	"testing"
)

func TestSplitSeedNodes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "redis-0:6379", []string{"redis-0:6379"}},
		{"comma list", "a:6379,b:6379,c:6379", []string{"a:6379", "b:6379", "c:6379"}},
		{"whitespace trimmed", " a:6379 , b:6379 ", []string{"a:6379", "b:6379"}},
		{"redis:// scheme stripped", "redis://a:6379,redis://b:6379", []string{"a:6379", "b:6379"}},
		{"empty entries dropped", "a:6379,,b:6379,", []string{"a:6379", "b:6379"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSeedNodes(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitSeedNodes(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
// <<< OPENFRAME(redis-seed-nodes)
