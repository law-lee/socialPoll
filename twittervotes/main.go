package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nsqio/go-nsq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	MongoDBName      = "ballots"
	MongoCollectName = "polls"
)

var db *mongo.Client

func dialdb() (context.Context, error) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return nil, err
	}
	return ctx, nil
}
func closedb(ctx context.Context) {
	if err := db.Disconnect(ctx); err != nil {
		panic(err)
	}
}

type poll struct {
	Options []string
}

// Our poll document contains more than just Options, but our program doesn't care about
//anything else, so there's no need for us to bloat our poll struct
func loadOptions() ([]string, error) {
	var options []string
	//  passing nil (meaning no filtering).
	// When we have a poll, we use the append method to build up the options slice. Of course,
	//with millions of polls in the database, this slice too would grow large and unwieldy. For
	//that kind of scale, we would probably run multiple twittervotes programs, each
	//dedicated to a portion of the poll data. A simple way to do this would be to break polls into
	//groups based on the letters the titles begin with, such as group A-N and O-Z. A somewhat
	//more sophisticated approach would be to add a field to the poll document, grouping it up
	//in a more controlled manner, perhaps based on the stats for the other groups so that we are
	//able to balance the load across many twittervotes instances.
	cur, err := db.Database(MongoDBName).Collection(MongoCollectName).Find(context.Background(), bson.D{})
	if err != nil {
		return nil, fmt.Errorf("mongodb find cursor err: %v", err)
	}
	var p poll
	//  This is a very memory-efficient way of reading the poll data because it only ever
	//uses a single poll object. If we were to use the All method instead, the amount of memory
	//we'd use would depend on the number of polls we had in our database, which could be out
	//of our control.
	for cur.Next(context.Background()) {
		err = cur.Decode(&p)
		if err != nil {
			return nil, fmt.Errorf("cur next err: %v", err)
		}
		options = append(options, p.Options...)
	}
	log.Printf("load options: %v", options)
	return options, nil
}

func publishVotes(votes <-chan string) <-chan struct{} {
	stopchan := make(chan struct{}, 1)
	pub, _ := nsq.NewProducer("localhost:4150",
		nsq.NewConfig())
	go func() {
		for vote := range votes {
			pub.Publish("votes", []byte(vote)) // publish vote
		}
		log.Println("Publisher: Stopping")
		pub.Stop()
		log.Println("Publisher: Stopped")
		stopchan <- struct{}{}
	}()
	return stopchan
}

func main() {
	var stoplock sync.Mutex // protects stop
	stop := false
	stopChan := make(chan struct{}, 1)
	signalChan := make(chan os.Signal, 1)
	go func() {
		<-signalChan
		stoplock.Lock()
		stop = true
		stoplock.Unlock()
		log.Println("Stopping...")
		stopChan <- struct{}{}
		closeConn()
	}()
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	ctx, err := dialdb()
	if err != nil {
		log.Fatalln("failed to dial MongoDB:", err)
	}
	defer closedb(ctx)

	// start things
	votes := make(chan string) // chan for votes
	publisherStoppedChan := publishVotes(votes)
	twitterStoppedChan := startTwitterStream(stopChan, votes)
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			closeConn()
			stoplock.Lock()
			if stop {
				stoplock.Unlock()
				return
			}
			stoplock.Unlock()
		}
	}()
	<-twitterStoppedChan
	close(votes)
	<-publisherStoppedChan
}
