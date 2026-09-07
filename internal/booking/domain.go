package booking

import (
	"errors"
	"time"
)

// Seat lifecycle statuses. A held seat carries a TTL; a confirmed one does not.
const (
	StatusHeld      = "held"
	StatusConfirmed = "confirmed"
)

var (
	ErrSeatAlreadybooked = errors.New("seat is already taken")
	ErrSessionNotFound   = errors.New("session not found or expired")
	ErrNotSessionOwner   = errors.New("session belongs to another user")
	ErrAlreadyConfirmed  = errors.New("session is already confirmed")
)

// Booking represents a seat hold or a confirmed seat reservation.
type Booking struct {
	ID        string    `json:"id"`
	MovieID   string    `json:"movie_id"`
	SeatID    string    `json:"seat_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BookingStore interface {
	Book(b Booking) error
	ListBookings(movieID string) []Booking
}

// SessionStore is the full hold/confirm/release lifecycle the HTTP API needs.
type SessionStore interface {
	Hold(b Booking) (Booking, error)
	Confirm(sessionID, userID string) (Booking, error)
	Release(sessionID, userID string) error
	ListBookings(movieID string) []Booking
}
