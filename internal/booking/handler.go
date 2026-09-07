package booking

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/MohitSagar/cinema-booking/internal/utils"
)

// Movie is a screening. Rows and SeatsPerRow are what the frontend uses to
// draw the seat grid, so seat ids run A1..{row}{SeatsPerRow}.
type Movie struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Rows        int    `json:"rows"`
	SeatsPerRow int    `json:"seats_per_row"`
}

var defaultMovies = []Movie{
	{ID: "inception", Title: "Inception", Rows: 5, SeatsPerRow: 8},
	{ID: "dune", Title: "Dune", Rows: 4, SeatsPerRow: 6},
}

type Handler struct {
	svc    *Service
	movies []Movie
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, movies: defaultMovies}
}

// Routes registers the API that static/index.html calls.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /movies", h.listMovies)
	mux.HandleFunc("GET /movies/{movieID}/seats", h.listSeats)
	mux.HandleFunc("POST /movies/{movieID}/seats/{seatID}/hold", h.holdSeat)
	mux.HandleFunc("PUT /sessions/{sessionID}/confirm", h.confirmSession)
	mux.HandleFunc("DELETE /sessions/{sessionID}", h.releaseSession)
}

// seatStatus describes one seat that is not available. Seats the frontend
// gets no entry for are drawn as free.
type seatStatus struct {
	SeatID    string `json:"seat_id"`
	Booked    bool   `json:"booked"`
	Confirmed bool   `json:"confirmed"`
	UserID    string `json:"user_id"`
}

type sessionResponse struct {
	SessionID string    `json:"session_id"`
	MovieID   string    `json:"movie_id"`
	SeatID    string    `json:"seat_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type userRequest struct {
	UserID string `json:"user_id"`
}

func (h *Handler) listMovies(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, http.StatusOK, h.movies)
}

func (h *Handler) listSeats(w http.ResponseWriter, r *http.Request) {
	movie, ok := h.movie(r.PathValue("movieID"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown movie")
		return
	}

	bookings := h.svc.Seats(movie.ID)
	statuses := make([]seatStatus, 0, len(bookings))
	for _, b := range bookings {
		statuses = append(statuses, seatStatus{
			SeatID:    b.SeatID,
			Booked:    true,
			Confirmed: b.Status == StatusConfirmed,
			UserID:    b.UserID,
		})
	}

	utils.WriteJSON(w, http.StatusOK, statuses)
}

func (h *Handler) holdSeat(w http.ResponseWriter, r *http.Request) {
	movie, ok := h.movie(r.PathValue("movieID"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown movie")
		return
	}

	seatID := r.PathValue("seatID")
	if !validSeat(movie, seatID) {
		writeError(w, http.StatusBadRequest, "no seat "+seatID+" in this screen")
		return
	}

	req, err := decodeUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	held, err := h.svc.Hold(Booking{MovieID: movie.ID, SeatID: seatID, UserID: req.UserID})
	if err != nil {
		writeStoreError(w, err)
		return
	}

	utils.WriteJSON(w, http.StatusCreated, newSessionResponse(held))
}

func (h *Handler) confirmSession(w http.ResponseWriter, r *http.Request) {
	req, err := decodeUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	confirmed, err := h.svc.Confirm(r.PathValue("sessionID"), req.UserID)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	utils.WriteJSON(w, http.StatusOK, newSessionResponse(confirmed))
}

func (h *Handler) releaseSession(w http.ResponseWriter, r *http.Request) {
	req, err := decodeUser(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.svc.Release(r.PathValue("sessionID"), req.UserID); err != nil {
		writeStoreError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) movie(id string) (Movie, bool) {
	for _, m := range h.movies {
		if m.ID == id {
			return m, true
		}
	}

	return Movie{}, false
}

func newSessionResponse(b Booking) sessionResponse {
	return sessionResponse{
		SessionID: b.ID,
		MovieID:   b.MovieID,
		SeatID:    b.SeatID,
		UserID:    b.UserID,
		Status:    b.Status,
		ExpiresAt: b.ExpiresAt,
	}
}

func decodeUser(r *http.Request) (userRequest, error) {
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, errors.New("body must be JSON with a user_id")
	}
	if req.UserID == "" {
		return req, errors.New("user_id is required")
	}

	return req, nil
}

// validSeat keeps arbitrary seat ids from creating keys for seats that do not
// exist in the screen.
func validSeat(m Movie, seatID string) bool {
	if len(seatID) < 2 {
		return false
	}

	row := seatID[0]
	if row < 'A' || int(row-'A') >= m.Rows {
		return false
	}

	num, err := strconv.Atoi(seatID[1:])

	return err == nil && num >= 1 && num <= m.SeatsPerRow
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSeatAlreadybooked), errors.Is(err, ErrAlreadyConfirmed):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrSessionNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrNotSessionOwner):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		log.Printf("booking: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// writeError uses the {"error": "..."} shape the frontend reads messages from.
func writeError(w http.ResponseWriter, status int, msg string) {
	utils.WriteJSON(w, status, map[string]string{"error": msg})
}
