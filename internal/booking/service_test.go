package booking

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	redisadapter "github.com/MohitSagar/cinema-booking/internal/adapters/redis"
)

func TestConcurrentBooking_ExactlyOneWins(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)

	const numGoroutines = 100_000 // 100k users trying to book a seat at the same time

	var (
		successes atomic.Int64
		failures  atomic.Int64
		wg        sync.WaitGroup
	)

	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(userNum int) {
			defer wg.Done()
			err := svc.Book(Booking{
				MovieID: "screen-1",
				SeatID:  "A1",
				UserID:  uuid.New().String(),
			})
			if err == nil {
				successes.Add(1)
			} else {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Errorf("expected exactly 1 success, got %d", got)
	}
	if got := failures.Load(); got != int64(numGoroutines-1) {
		t.Errorf("expected %d failures, got %d", numGoroutines-1, got)
	}
}

// TestRedisStore_ExactlyOneWins exercises the Redis-backed store.
// Requires `docker compose up`; skipped when redis is unreachable.
func TestRedisStore_ExactlyOneWins(t *testing.T) {
	const addr = "127.0.0.1:6379"

	// probe first: redisadapter.NewClient calls log.Fatalf on a failed ping,
	// which would kill the whole test binary instead of failing this test.
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	conn.Close()

	rdb := redisadapter.NewClient(addr)
	defer rdb.Close()

	store := NewRedisStore(rdb)
	svc := NewService(store)

	// unique movie id per run so leftover holds from an earlier run
	// (TTL is 2 minutes) don't make every attempt fail
	movieID := "screen-" + uuid.New().String()

	const numGoroutines = 1_000

	var (
		successes atomic.Int64
		failures  atomic.Int64
		wg        sync.WaitGroup
	)

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			err := svc.Book(Booking{
				MovieID: movieID,
				SeatID:  "A1",
				UserID:  uuid.New().String(),
			})
			if err == nil {
				successes.Add(1)
			} else {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Errorf("expected exactly 1 success, got %d", got)
	}
	if got := failures.Load(); got != int64(numGoroutines-1) {
		t.Errorf("expected %d failures, got %d", numGoroutines-1, got)
	}
}
