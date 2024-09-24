package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

func main() {
	delayTime, _ := strconv.Atoi(os.Getenv("TOGGLE_SERVICE_DELAY"))

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}

	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

	applicationPort := os.Getenv("APPLICATION_PORT")
	if applicationPort == "" {
		applicationPort = "5000"
	}

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisHost + ":" + redisPort,
		Password: "",
		DB:       0,
	})

	// Initialize router
	mux := http.NewServeMux()

	// Define routes
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		slog.Info("Main request successful")
		fmt.Fprintf(w, "Hello World!")
	})

	mux.HandleFunc("GET /favorites", func(w http.ResponseWriter, req *http.Request) {
		// artificial sleep for delayTime
		time.Sleep(time.Duration(delayTime) * time.Millisecond)

		userID := req.URL.Query().Get("user_id")

		slog.Info("Getting favorites", "user", userID)

		favorites, err := rdb.SMembers(req.Context(), userID).Result()
		if err != nil {
			slog.Error("Failed to get favorites", "user", userID)

			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to get favorites")
			return
		}

		slog.Info("Found favorites", "user", userID, "favorites", favorites)
		err = json.NewEncoder(w).Encode(map[string]any{
			"favorites": favorites,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to encode favorites")
		}
	})

	mux.HandleFunc("POST /favorites", func(w http.ResponseWriter, req *http.Request) {
		// artificial sleep for delayTime
		time.Sleep(time.Duration(delayTime) * time.Millisecond)

		userID := req.URL.Query().Get("user_id")

		slog.Info("Adding or removing favorites", "user", userID)

		var data struct {
			ID int `json:"id"`
		}
		err := json.NewDecoder(req.Body).Decode(&data)
		if err != nil {
			slog.Error("Failed to decode request body", "user", userID)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Failed to decode request body")
			return
		}

		redisResponse := rdb.SRem(req.Context(), userID, data.ID)
		if redisResponse.Err() != nil {
			slog.Error("Failed to remove movie from favorites", "user", userID)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to remove movie from favorites")
			return
		}

		if redisResponse.Val() == 0 {
			rdb.SAdd(req.Context(), userID, data.ID)
		}

		favorites, err := rdb.SMembers(req.Context(), userID).Result()
		slog.Info("Getting favorites", "user", userID)
		if err != nil {
			slog.Error("Failed to get favorites", "user", userID)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to get favorites")
			return
		}

		slog.Info("Found favorites", "user", userID, "favorites", favorites)

		// if enabled, in 50% of the cases, sleep for 2 seconds
		sleepTimeStr := os.Getenv("TOGGLE_CANARY_DELAY")
		sleepTime := 0
		if sleepTimeStr != "" {
			sleepTime, _ = strconv.Atoi(sleepTimeStr)
		}

		if sleepTime > 0 && rand.Float64() < 0.5 {
			time.Sleep(time.Duration(rand.NormFloat64()*float64(sleepTime/10)+float64(sleepTime)) * time.Millisecond)
			// add label to transaction
			slog.Info("Canary enabled")

			// read env var TOGGLE_CANARY_FAILURE, which is a float between 0 and 1
			if toggleCanaryFailureStr := os.Getenv("TOGGLE_CANARY_FAILURE"); toggleCanaryFailureStr != "" {
				toggleCanaryFailure, err := strconv.ParseFloat(toggleCanaryFailureStr, 64)
				if err != nil {
					toggleCanaryFailure = 0
				}
				if rand.Float64() < toggleCanaryFailure {
					// throw an exception in 50% of the cases
					slog.Error("Something went wrong")
					panic("Something went wrong")
				}
			}
		}

		err = json.NewEncoder(w).Encode(map[string]any{
			"favorites": favorites,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed to encode favorites")
		}
	})

	// Start server
	slog.Info("App startup")
	log.Fatal(http.ListenAndServe(":"+applicationPort, mux))
	slog.Info("App stopped")
}
