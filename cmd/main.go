package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	redisadapter "github.com/MohitSagar/cinema-booking/internal/adapters/redis"
	"github.com/MohitSagar/cinema-booking/internal/booking"
)

func main() {
	addr := envOr("REDIS_ADDR", "127.0.0.1:6379")

	rdb := redisadapter.NewClient(addr)
	defer rdb.Close()

	svc := booking.NewService(booking.NewRedisStore(rdb))

	mux := http.NewServeMux()
	booking.NewHandler(svc).Routes(mux)
	mux.Handle("GET /", http.FileServer(http.Dir(staticDir())))

	listen := envOr("LISTEN_ADDR", ":8080")
	log.Printf("listening on http://localhost%s", listen)
	if err := http.ListenAndServe(listen, mux); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// staticDir locates the frontend whether main is run from the repo root
// (go run ./cmd) or from inside cmd/ (go run main.go).
func staticDir() string {
	for _, dir := range []string{"static", filepath.Join("..", "static")} {
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
			return dir
		}
	}

	return "static"
}
