package avatar

import (
	"context"
	"fmt"
)

// SumActiveSizeBytes returns the total size, in bytes, of all non-deleted
// avatar originals. It backs the avatars_storage_bytes business gauge, sampled
// periodically by the server rather than at scrape time.
//
// COALESCE keeps the result 0 (not NULL) when no rows match, so the caller
// never has to special-case an empty table.
func (r *PostgresRepository) SumActiveSizeBytes(ctx context.Context) (int64, error) {
	const query = `SELECT COALESCE(SUM(size_bytes), 0) FROM avatars WHERE deleted_at IS NULL`
	var total int64
	if err := r.pool.QueryRow(ctx, query).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum active avatar size: %w", err)
	}
	return total, nil
}
