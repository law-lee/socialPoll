package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestMongodbAndNSQ(t *testing.T) {
	ctx, err := dialdb()
	if err != nil {
		log.Fatalln("failed to dial MongoDB:", err)
	}
	defer closedb(ctx)
	votes := make(chan string) // chan for votes
	publisherStoppedChan := publishVotes(votes)
	options, err := loadOptions()
	if err != nil {
		t.Fatalf("load options err: %v", err)
	}
	testVotes := []string{"i am happy with x", "i am sad for you!", "we are win"}
	for _, text := range testVotes {
		for _, option := range options {
			if strings.Contains(
				strings.ToLower(text),
				strings.ToLower(option),
			) {
				log.Println("vote:", option)
				votes <- option
			}
		}
	}

	close(votes)
	<-publisherStoppedChan
}

func TestTwitter(t *testing.T) {
	options := []string{"happy", "win", "fail"}
	u, err := url.Parse("https://api.twitter.com/2/tweets/" + TwitterID)
	if err != nil {
		log.Println("creating filter request failed:", err)
		return
	}
	query := make(url.Values)
	query.Set("track", strings.Join(options, ","))
	req, err := http.NewRequest("GET", u.String(), strings.NewReader(query.Encode()))
	if err != nil {
		log.Println("creating filter request failed:", err)
		return
	}
	resp, err := makeRequest(req, query)
	if err != nil {
		log.Println("making request failed:", err)
		return
	}
	reader := resp.Body
	t.Logf("reader: %v", reader)
	decoder := json.NewDecoder(reader)
	var tt tweet
	if err := decoder.Decode(&tt); err != nil {
		t.Fatalf("decode tweet err: %v", err)
	}
	t.Log(tt.Text)
}
