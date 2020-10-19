package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Participants struct {
	Name  string `json:"Name"`
	Email string `json:"Email"`
	RSVP  string `json:"RSVP"`
}

type Meeting struct {
	Id                string         `json:"Id"`
	Title             string         `json:"Title"`
	Participants      []Participants `json:"Participants"`
	StartTime         time.Time      `json:"Start Time"`
	EndTime           time.Time      `json:"End Time"`
	CreationTimestamp time.Time      `json:"Creation Timestamp"`
}

type meetingScheduler struct {
	sync.Mutex
	store map[string]Meeting
}

func (h *meetingScheduler) meetings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.get(w, r)
		return
	case "POST":
		h.post(w, r)
		return
	default:
		w.Write([]byte("method not allowed"))
		return
	}
}

func (h *meetingScheduler) get(w http.ResponseWriter, r *http.Request) {
	meetings := make([]Meeting, len(h.store))

	i := 0
	for _, meeting := range h.store {
		meetings[i] = meeting
		i++
	}
	h.Unlock()

	jsonBytes, err := json.Marshal(meetings)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}

	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func (h *meetingScheduler) post(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	ct := r.Header.Get("content-type")
	if ct != "application/json" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		w.Write([]byte(fmt.Sprintf("need content-type 'application/json', but got '%s'", ct)))
		return
	}

	var meeting Meeting
	err = json.Unmarshal(bodyBytes, &meeting)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	meeting.Id = fmt.Sprintf("%d", time.Now().UnixNano())
	h.Lock()
	h.store[meeting.Id] = meeting
	defer h.Unlock()
}

func (h *meetingScheduler) getRandomMeeting(w http.ResponseWriter, r *http.Request) {
	ids := make([]string, len(h.store))
	h.Lock()
	i := 0
	for id := range h.store {
		ids[i] = id
		i++
	}
	defer h.Unlock()

	var target string
	if len(ids) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	} else if len(ids) == 1 {
		target = ids[0]
	} else {
		rand.Seed(time.Now().UnixNano())
		target = ids[rand.Intn(len(ids))]
	}

	w.Header().Add("location", fmt.Sprintf("/meetings/%s", target))
	w.WriteHeader(http.StatusFound)
}

func (h *meetingScheduler) getMeeting(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.String(), "/")
	if len(parts) != 3 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if parts[0] == "random" {
		h.getRandomMeeting(w, r)
		return
	}

	h.Lock()
	meeting, ok := h.store[parts[0]]
	h.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	jsonBytes, err := json.Marshal(meeting)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
	}

	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBytes)
}

func newMeetingScheduler() *meetingScheduler {
	return &meetingScheduler{
		store: map[string]Meeting{},
	}
}

func main() {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	defer func() {
		if err = client.Disconnect(ctx); err != nil {
			panic(err)
		}
	}()

	meetingScheduler := newMeetingScheduler()
	http.HandleFunc("/meetings", meetingScheduler.post)
	http.HandleFunc("/meeting/", meetingScheduler.getMeeting)
	err1 := http.ListenAndServe(":8080", nil)
	if err1 != nil {
		panic(err1)
	}
}
