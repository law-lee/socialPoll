package main

import (
	"net/http"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type poll struct {
	ID      string   `bson:"_id" json:"id"`
	Title   string   `json:"title"`
	Options []string `json:"options"`

	Results map[string]int `json:"results,omitempty"`
	APIKey  string         `json:"apikey"`
}

func (s *Server) handlePolls(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.handlePollsGet(w, r)
		return
	case "POST":
		s.handlePollsPost(w, r)
		return
	case "DELETE":
		s.handlePollsDelete(w, r)
		return
	case "OPTIONS":
		w.Header().Add("Access-Control-Allow-Methods", "DELETE")
		respond(w, r, http.StatusOK, nil)
		return
	}
	// not found
	respondHTTPErr(w, r, http.StatusNotFound)
}

func (s *Server) handlePollsGet(w http.ResponseWriter, r *http.Request) {
	c := s.db.Database("ballots").Collection("polls")
	var q *mongo.Cursor
	var err error
	p := NewPath(r.URL.Path)
	if p.HasID() {
		// get specific poll
		q, err = c.Find(s.ctx, bson.M{"_id": p.ID})
		if err != nil {
			respondErr(w, r, http.StatusInternalServerError, "get record from mongodb err ", err)
			return
		}
	} else {
		// get all polls
		q, err = c.Find(s.ctx, bson.M{})
		if err != nil {
			respondErr(w, r, http.StatusInternalServerError, "get all record from mongodb err ", err)
			return
		}
	}
	var result []*poll
	if err := q.All(s.ctx, &result); err != nil {
		respondErr(w, r, http.StatusInternalServerError, "unmarshal result err ", err)
		return
	}
	respond(w, r, http.StatusOK, &result)
}
func (s *Server) handlePollsPost(w http.ResponseWriter, r *http.Request) {
	c := db.Database("ballots").Collection("polls")
	var p poll
	if err := decodeBody(r, &p); err != nil {
		respondErr(w, r, http.StatusBadRequest, "failed to read poll from request", err)
		return
	}
	apikey, ok := APIKey(r.Context())
	if ok {
		p.APIKey = apikey
	}
	p.ID = primitive.NewObjectID().Hex()
	if _, err := c.InsertOne(s.ctx, p); err != nil {
		respondErr(w, r, http.StatusInternalServerError,
			"failed to insert poll", err)
		return
	}
	w.Header().Set("Location", "polls/"+p.ID)
	respond(w, r, http.StatusCreated, nil)
}
func (s *Server) handlePollsDelete(w http.ResponseWriter, r *http.Request) {
	c := db.Database("ballots").Collection("polls")
	p := NewPath(r.URL.Path)
	if !p.HasID() {
		respondErr(w, r, http.StatusMethodNotAllowed, "Cannot delete all polls.")
		return
	}
	if _, err := c.DeleteOne(s.ctx, bson.M{"_id": p.ID}); err != nil {
		respondErr(w, r, http.StatusInternalServerError, "failed to delete poll", err)
		return
	}
	respond(w, r, http.StatusOK, nil) // ok
}
