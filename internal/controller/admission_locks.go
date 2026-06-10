package controller

import (
	"context"
	"sort"

	"github.com/openziti/agora/internal/persistence"
)

const (
	advisoryLockScopeAccount     = 1
	advisoryLockScopeEnvironment = 2
	advisoryLockScopeTunnel      = 3
)

func lockAccountScope(ctx context.Context, q persistence.Queryer, accountID string) error {
	return advisoryTransactionLock(ctx, q, advisoryLockScopeAccount, accountID)
}

func lockEnvironmentScope(ctx context.Context, q persistence.Queryer, environmentID string) error {
	return advisoryTransactionLock(ctx, q, advisoryLockScopeEnvironment, environmentID)
}

func lockTunnelScope(ctx context.Context, q persistence.Queryer, tunnelID string) error {
	return advisoryTransactionLock(ctx, q, advisoryLockScopeTunnel, tunnelID)
}

func lockTunnelScopes(ctx context.Context, q persistence.Queryer, tunnelIDs []string) error {
	ids := uniqueStrings(tunnelIDs)
	sort.Strings(ids)
	for _, id := range ids {
		if err := lockTunnelScope(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

func advisoryTransactionLock(ctx context.Context, q persistence.Queryer, scope int, key string) error {
	_, err := q.ExecContext(ctx, `select pg_advisory_xact_lock($1::integer, hashtext($2))`, scope, key)
	return err
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
