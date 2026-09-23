package mysqlredis

import (
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// OpenTelemetry instruments for the host lookup cache (covers both
// LoadHostByNodeKey and LoadHostByOrbitNodeKey). These are unused until the
// cache is wired up via WithHostCache. When the global MeterProvider is not
// configured (tests, services started without OTEL), otel.Meter returns a
// no-op meter and all operations on these counters silently succeed.

var (
	meter = otel.Meter("fleet")

	// hostCacheLookups counts cache read attempts, labeled by result.
	// Attribute `result` is one of: hit, negative_hit, miss.
	hostCacheLookups metric.Int64Counter

	// hostCacheErrors counts Redis/JSON errors encountered in the cache path,
	// labeled by operation. Attribute `op` is one of: get, set, del.
	hostCacheErrors metric.Int64Counter

	// hostCacheInvalidations counts cache invalidation operations, labeled by
	// the write path that triggered the invalidation. Attribute `reason` is
	// one of: update, enroll, team, delete, cert.
	hostCacheInvalidations metric.Int64Counter
)

func init() {
	noopMeter := noop.NewMeterProvider().Meter("fleet")

	var err error
	hostCacheLookups, err = meter.Int64Counter(
		"fleet.host_cache.lookups",
		metric.WithDescription("Host lookup cache reads, labeled by result"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		log.Printf("mysqlredis: failed to initialize fleet.host_cache.lookups counter, falling back to no-op: %v", err)
		hostCacheLookups, _ = noopMeter.Int64Counter("fleet.host_cache.lookups")
	}

	hostCacheErrors, err = meter.Int64Counter(
		"fleet.host_cache.errors",
		metric.WithDescription("Host lookup cache Redis/serialization errors, labeled by operation"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		log.Printf("mysqlredis: failed to initialize fleet.host_cache.errors counter, falling back to no-op: %v", err)
		hostCacheErrors, _ = noopMeter.Int64Counter("fleet.host_cache.errors")
	}

	hostCacheInvalidations, err = meter.Int64Counter(
		"fleet.host_cache.invalidations",
		metric.WithDescription("Host lookup cache invalidations, labeled by the write path reason"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		log.Printf("mysqlredis: failed to initialize fleet.host_cache.invalidations counter, falling back to no-op: %v", err)
		hostCacheInvalidations, _ = noopMeter.Int64Counter("fleet.host_cache.invalidations")
	}
}

func hostCacheLookupAttrs(result string) metric.AddOption {
	return metric.WithAttributes(attribute.String("result", result))
}

func hostCacheErrorAttrs(op string) metric.AddOption {
	return metric.WithAttributes(attribute.String("op", op))
}

func hostCacheInvalidationAttrs(reason string) metric.AddOption {
	return metric.WithAttributes(attribute.String("reason", reason))
}
