package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const defaultHoldTTL = 2 * time.Minute

// RedisStore implements session-based seat booking backed by Redis
// key design:
// seat:{movieID}:{seatID}  --> session JSON (TTL = held , no TTL = confirmed)
// session:{sessionID}  --> seat key (reverse lookup)

type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

// seatKey builds the key that a single seat in a screening is stored under
func seatKey(movieID, seatID string) string {
	return fmt.Sprintf("seat:%s:%s", movieID, seatID)
}

// sessionKey builds the reverse-lookup key for a session
func sessionKey(id string) string {
	return fmt.Sprintf("session:%s", id)
}

// confirmScript promotes a held seat to confirmed: it rewrites the seat JSON
// without a TTL and drops the reverse-lookup key. Read-modify-write has to run
// server-side, otherwise two concurrent confirms can both read "held" and win.
var confirmScript = redis.NewScript(`
local seat = redis.call('GET', KEYS[1])
if not seat then return 'ERR_NO_SESSION' end
local raw = redis.call('GET', seat)
if not raw then return 'ERR_NO_SESSION' end
local b = cjson.decode(raw)
if b['id'] ~= ARGV[1] then return 'ERR_NO_SESSION' end
if b['user_id'] ~= ARGV[2] then return 'ERR_NOT_OWNER' end
if b['status'] == ARGV[3] then return 'ERR_CONFIRMED' end
b['status'] = ARGV[3]
b['expires_at'] = ARGV[4]
local updated = cjson.encode(b)
redis.call('SET', seat, updated)
redis.call('DEL', KEYS[1])
return updated
`)

// releaseScript frees a held seat. A confirmed seat is not releasable.
var releaseScript = redis.NewScript(`
local seat = redis.call('GET', KEYS[1])
if not seat then return 'ERR_NO_SESSION' end
local raw = redis.call('GET', seat)
if raw then
  local b = cjson.decode(raw)
  if b['id'] ~= ARGV[1] then return 'ERR_NO_SESSION' end
  if b['user_id'] ~= ARGV[2] then return 'ERR_NOT_OWNER' end
  if b['status'] == ARGV[3] then return 'ERR_CONFIRMED' end
  redis.call('DEL', seat)
end
redis.call('DEL', KEYS[1])
return 'OK'
`)

// Hold claims a seat for defaultHoldTTL. SET NX is the whole concurrency
// story: exactly one caller can create the key, everyone else is rejected.
func (s *RedisStore) Hold(b Booking) (Booking, error) {
	ctx := context.Background()

	b.ID = uuid.New().String()
	b.Status = StatusHeld
	b.ExpiresAt = time.Now().Add(defaultHoldTTL)

	val, err := json.Marshal(b)
	if err != nil {
		return Booking{}, err
	}

	key := seatKey(b.MovieID, b.SeatID)
	res, err := s.rdb.SetArgs(ctx, key, val, redis.SetArgs{
		Mode: "NX", // set if not exists
		TTL:  defaultHoldTTL,
	}).Result()
	switch {
	case errors.Is(err, redis.Nil):
		// NX declined: the seat is already held or confirmed
		return Booking{}, ErrSeatAlreadybooked
	case err != nil:
		return Booking{}, fmt.Errorf("hold %s: %w", key, err)
	case res != "OK":
		return Booking{}, ErrSeatAlreadybooked
	}

	if err := s.rdb.Set(ctx, sessionKey(b.ID), key, defaultHoldTTL).Err(); err != nil {
		// don't leave behind a hold that nobody can confirm or release
		s.rdb.Del(ctx, key)
		return Booking{}, fmt.Errorf("record session %s: %w", b.ID, err)
	}

	return b, nil
}

func (s *RedisStore) Confirm(sessionID, userID string) (Booking, error) {
	res, err := confirmScript.Run(context.Background(), s.rdb,
		[]string{sessionKey(sessionID)},
		sessionID, userID, StatusConfirmed, time.Time{}.Format(time.RFC3339Nano),
	).Text()
	if err != nil {
		return Booking{}, fmt.Errorf("confirm %s: %w", sessionID, err)
	}
	if err := scriptError(res); err != nil {
		return Booking{}, err
	}

	return parseSession(res)
}

func (s *RedisStore) Release(sessionID, userID string) error {
	res, err := releaseScript.Run(context.Background(), s.rdb,
		[]string{sessionKey(sessionID)},
		sessionID, userID, StatusConfirmed,
	).Text()
	if err != nil {
		return fmt.Errorf("release %s: %w", sessionID, err)
	}

	return scriptError(res)
}

func (s *RedisStore) ListBookings(movieID string) []Booking {
	ctx := context.Background()
	pattern := seatKey(movieID, "*")

	// never nil: the frontend calls .forEach on this and a JSON null would throw
	sessions := []Booking{}

	iter := s.rdb.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		val, err := s.rdb.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}
		session, err := parseSession(val)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions
}

// scriptError maps the sentinel strings the Lua scripts return onto errors.
func scriptError(res string) error {
	switch res {
	case "ERR_NO_SESSION":
		return ErrSessionNotFound
	case "ERR_NOT_OWNER":
		return ErrNotSessionOwner
	case "ERR_CONFIRMED":
		return ErrAlreadyConfirmed
	}

	return nil
}

func parseSession(val string) (Booking, error) {
	var data Booking
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return Booking{}, err
	}

	return data, nil
}
