package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type SummaryEvent struct {
	Calendar       string  `json:"calendar"`
	Summary        string  `json:"summary"`
	Created        string  `json:"created"`
	RecurringEvent bool    `json:"recurringEvent"`
	EventTime      float64 `json:"eventTime"`
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {

	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	r := mux.NewRouter()
	r.HandleFunc("/", SayHelloFunc).Methods(http.MethodGet)
	r.HandleFunc("/calendar", CalendarHandler).Methods(http.MethodGet)

	srv := &http.Server{
		Addr: ":8080",
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r, // Pass our instance of gorilla/mux in.
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("shutting down")
	os.Exit(0)
}

func CalendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		c := make([]SummaryEvent, 0)

		ctx := context.Background()
		b, err := ioutil.ReadFile("resources\\credentials.json")
		if err != nil {
			log.Fatalf("Unable to read client secret file: %v", err)
		}

		// If modifying these scopes, delete your previously saved token.json.
		config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
		if err != nil {
			log.Fatalf("Unable to parse client secret file to config: %v", err)
		}
		client := getClient(config)

		srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			log.Fatalf("Unable to retrieve Calendar client: %v", err)
		}

		cal, err := srv.CalendarList.List().MinAccessRole("owner").MaxResults(20).Do()

		if err != nil {
			log.Fatalf("Unable to retrieve users Calenders: %v", err)
		}

		if len(cal.Items) == 0 {
			fmt.Printf("No calendars found")
		} else {

			for _, userCalendar := range cal.Items {

				events, err := srv.Events.List(userCalendar.Id).SingleEvents(true).ShowDeleted(false).TimeMin(time.Now().AddDate(0, -1, 0).Format(time.RFC3339)).TimeMax(time.Now().Format(time.RFC3339)).OrderBy("updated").Do()

				if err != nil {
					log.Fatalf("Unable to retrieve events from the Calendar %v", err)
				} else {
					for _, event := range events.Items {
						summary := event.Summary

						endTime, err := time.Parse(time.RFC3339, event.End.DateTime)
						if err != nil {
							log.Fatalf("Error parsing time from event, %s", err)
						}

						startTime, err := time.Parse(time.RFC3339, event.Start.DateTime)
						if err != nil {
							log.Fatalf("Error parsing time from event, %s", err)
						}

						time := endTime.Sub(startTime)

						var calEvent = SummaryEvent{
							Calendar:  userCalendar.Summary,
							Summary:   summary,
							Created:   event.Created,
							EventTime: time.Minutes(),
						}

						c = append(c, calEvent)
					}
				}
			}

			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(c); err != nil {
				log.Fatalf("Error parsing json from request %v", err)
			}
		}
	}
}

func SayHelloFunc(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello!"))
}
