package main

import (
	"context"
	"flag"
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

const updateDuration = 1 * time.Second

var fatalErr error
var db *mongo.Client

func fatal(e error) {
	fmt.Println(e)
	flag.PrintDefaults()
	fatalErr = e
}

func closedb(ctx context.Context) {
	log.Println("Closing database connection...")
	if err := db.Disconnect(ctx); err != nil {
		panic(err)
	}
}
func main() {
	var counts map[string]int
	var countsLock sync.Mutex
	//  Note that only when our main function exits will the
	//deferred function run, which in turn calls os.Exit(1) to exit the program with an exit
	//code of 1
	//Because the deferred statements are run in LIFO (last in, first out) order, the first
	//function we defer will be the last function to be executed, which is why the first thing we do
	//in the main function is defer the exiting code. This allows us to be sure that other functions
	//we defer will be called before the program exits.
	defer func() {
		if fatalErr != nil {
			os.Exit(1)
		}
	}()
	log.Println("Connecting to database...")
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		fatal(err)
		return
	}
	defer closedb(context.Background())
	pollData := db.Database("ballots").Collection("polls")
	log.Println("Connecting to nsq...")
	q, err := nsq.NewConsumer("votes", "counter", nsq.NewConfig())
	if err != nil {
		fatal(err)
		return
	}
	q.AddHandler(nsq.HandlerFunc(func(m *nsq.Message) error {
		countsLock.Lock()
		defer countsLock.Unlock()
		if counts == nil {
			counts = make(map[string]int)
		}
		vote := string(m.Body)
		counts[vote]++
		return nil
	}))
	if err = q.ConnectToNSQLookupd("localhost:4161"); err != nil {
		fatal(err)
		return
	}
	ticker := time.NewTicker(updateDuration)
	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		select {
		case <-ticker.C:
			doCount(&countsLock, &counts, pollData)
		case <-termChan:
			ticker.Stop()
			q.Stop()
		case <-q.StopChan:
			// finished
			return
		}
	}
}

func doCount(countsLock *sync.Mutex, counts *map[string]int, pollData *mongo.Collection) {
	countsLock.Lock()
	defer countsLock.Unlock()
	if len(*counts) == 0 {
		log.Println("No new votes, skipping database update")
		return
	}
	log.Println("Updating database...")
	log.Println(*counts)
	ok := true
	for option, count := range *counts {
		sel := bson.M{"options": bson.M{"$in": []string{option}}}
		up := bson.M{"$inc": bson.M{"results." + option: count}}
		if _, err := pollData.UpdateMany(context.Background(), sel, up); err != nil {
			log.Println("failed to update:", err)
			ok = false
		}
	}
	if ok {
		log.Println("Finished updating database...")
		*counts = nil // reset counts
	}
}
