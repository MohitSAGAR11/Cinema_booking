package booking

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type ConcurrentStore struct {
	// "seat:{movieID}:{seatID}" --> booking
	bookings map[string]Booking
	// sessionID --> seat key
	sessions map[string]string
	sync.RWMutex
}

func NewConcurrentStore() *ConcurrentStore {
	return &ConcurrentStore{
		bookings: map[string]Booking{},
		sessions: map[string]string{},
	}
}

// Book holds a seat and discards the session details.
func (s *ConcurrentStore) Book(b Booking) error {
	_, err := s.Hold(b)
	return err
}

func (s *ConcurrentStore) Hold(b Booking) (Booking, error) {
	s.Lock()
	defer s.Unlock()

	key := seatKey(b.MovieID, b.SeatID)
	if existing, exists := s.bookings[key]; exists && !expired(existing) {
		return Booking{}, ErrSeatAlreadybooked
	}

	b.ID = uuid.New().String()
	b.Status = StatusHeld
	b.ExpiresAt = time.Now().Add(defaultHoldTTL)

	s.bookings[key] = b
	s.sessions[b.ID] = key

	return b, nil
}

func (s *ConcurrentStore) Confirm(sessionID, userID string) (Booking, error) {
	s.Lock()
	defer s.Unlock()

	key, b, err := s.lookup(sessionID, userID)
	if err != nil {
		return Booking{}, err
	}
	if b.Status == StatusConfirmed {
		return Booking{}, ErrAlreadyConfirmed
	}

	b.Status = StatusConfirmed
	b.ExpiresAt = time.Time{}
	s.bookings[key] = b
	delete(s.sessions, sessionID)

	return b, nil
}

func (s *ConcurrentStore) Release(sessionID, userID string) error {
	s.Lock()
	defer s.Unlock()

	key, b, err := s.lookup(sessionID, userID)
	if err != nil {
		return err
	}
	if b.Status == StatusConfirmed {
		return ErrAlreadyConfirmed
	}

	delete(s.bookings, key)
	delete(s.sessions, sessionID)

	return nil
}

func (s *ConcurrentStore) ListBookings(movieID string) []Booking {
	s.RLock()
	defer s.RUnlock()

	// never nil: the frontend calls .forEach on this and a JSON null would throw
	result := []Booking{}
	for _, b := range s.bookings {
		if b.MovieID == movieID && !expired(b) {
			result = append(result, b)
		}
	}

	return result
}

// lookup resolves a session to its seat, checking ownership and expiry.
// Callers must already hold the lock.
func (s *ConcurrentStore) lookup(sessionID, userID string) (string, Booking, error) {
	key, ok := s.sessions[sessionID]
	if !ok {
		return "", Booking{}, ErrSessionNotFound
	}

	b, ok := s.bookings[key]
	if !ok || b.ID != sessionID || expired(b) {
		return "", Booking{}, ErrSessionNotFound
	}
	if b.UserID != userID {
		return "", Booking{}, ErrNotSessionOwner
	}

	return key, b, nil
}

// expired reports whether a hold's TTL has passed. Confirmed seats never expire.
func expired(b Booking) bool {
	return b.Status == StatusHeld && !b.ExpiresAt.IsZero() && time.Now().After(b.ExpiresAt)
}
