package main

import (
	"context"
	"fmt"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/badger/pb"
	"github.com/golang/protobuf/jsonpb"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	DB *badger.DB
)

func badgerGet(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	log.Printf("GET key: %s \n", key)
	if strings.Index(key, "secret:") == 0 {
		w.WriteHeader(http.StatusForbidden)
		log.Printf("secrets forbidden \n")
		return
	}
	data, err := getValue(key)
	switch err {
	case nil:
		w.WriteHeader(http.StatusOK)
	case badger.ErrKeyNotFound:
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	fmt.Fprintf(w,"%s",data)
	log.Printf("Got Value: %s \n",data)
}

func badgerPut(w http.ResponseWriter, r *http.Request) {
	//ToDo: implement TTL https://github.com/dgraph-io/badger#setting-time-to-livettl-and-user-metadata-on-keys
	// ?? use header TTL=Value ??
	data, err := ioutil.ReadAll(r.Body)
	if err != nil { panic (err) }
	key := mux.Vars(r)["key"]
	putValue(key,string(data))
	fmt.Fprintf(w,"ok")
	log.Printf("badger PUT Key: %s Value: %s \n",key,data)
}

func badgerStream(prefix string) *pb.KVList {
	stream := DB.NewStream()

	stream.NumGo = 1
	stream.Prefix = []byte(prefix)
	stream.LogPrefix = "Badger.Streaming"

	fullList := pb.KVList{}

	// Send is called serially, while Stream.Orchestrate is running.
	stream.Send = func(list *pb.KVList) error {
		fullList.Kv = append(fullList.Kv, list.Kv...)
		return nil
	}

	// Run the stream
	if err := stream.Orchestrate(context.Background()); err != nil {
		log.Printf("error: %s", err)
	}
	// Done.
	return &fullList
}

func streamrGet(w http.ResponseWriter, r *http.Request){
	prefix := mux.Vars(r)["prefix"]
	log.Printf("streamr GET prefix: %s", prefix)
	if strings.Index(prefix, "secret:") == 0 {
		w.WriteHeader(http.StatusForbidden)
		log.Printf("secrets forbidden \n")
		return
	}
	marshlr := &jsonpb.Marshaler{true,true,"  ",true,nil}
	marshlr.Marshal(w, badgerStream(prefix))
}

func secretPut(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil { panic(err) }
	key := mux.Vars(r)["key"]
  putValue("secret:"+key,string(data))
	fmt.Fprintf(w,"ok")
	log.Printf("secret PUT Key: %s Value: [REDACTED] \n",key)
}

func secretGet(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	data, err := getValue("secret:"+key)
	switch err {
	case nil:
		w.WriteHeader(http.StatusOK)
	case badger.ErrKeyNotFound:
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	fmt.Fprintf(w,"%s",data)
	log.Printf("secret GET Key: %s Value: [REDACTED] \n",key)
}

func putValue(key string, val string) {
	txn := DB.NewTransaction(true) // Read-write txn
	err := txn.Set([]byte(key), []byte(val))
	if err != nil {
		log.Fatal(err)
	}
	err = txn.Commit()
	if err != nil {
		log.Fatal(err)
	}
}

func getValue(key string) (value string, err error){
	txn := DB.NewTransaction(false)
	item, err := txn.Get([]byte(key))
	if err != nil {
		log.Print(err.Error() + " - " + key)
		return getDefault(key)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {log.Fatal(err)}
	return string(val), nil
}

func getDefault(key string) (value string, err error){
	txn := DB.NewTransaction(false)
	key = "default:"+key
	item, err := txn.Get([]byte(key))
	if err != nil {
		log.Print(err.Error() + " - " + key )
		return "", err
	}
	val, err := item.ValueCopy(nil)
	if err != nil {log.Fatal(err)}
	return string(val), nil
}

func putDefault(key string, val string) {
	txn := DB.NewTransaction(true) // Read-write txn
	key = "default:"+key
	err := txn.Set([]byte(key), []byte(val))
	if err != nil {
		log.Fatal(err)
	}
	err = txn.Commit()
	if err != nil {
		log.Fatal(err)
	}
}


func main() {

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func(){
		for sig := range c {
			log.Printf("Signal recieved: %s",sig)
			if (sig == os.Interrupt) || (sig == syscall.SIGTERM) {
				log.Print("Stopping Badger")
				DB.Close()
				log.Print("Exiting")
				os.Exit(0)

			}
		}
	}()

	go func() {
		ticker := time.NewTicker(60 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
		again:
			log.Print("Badger Garbage Collection - Start")
			err := DB.RunValueLogGC(0.7)
			log.Printf("Badger Garbage Collection - Finish. Err: %s",err)
			if err == nil {
				goto again
			}
		}
	}()

	var err error

	//Todo: move to /data/badger
	opts := badger.DefaultOptions("/tmp/badger")
	opts.ValueLogLoadingMode = options.FileIO
	opts.NumMemtables = 2
	opts.NumLevelZeroTables = 2
	opts.NumLevelZeroTablesStall = 4
	DB, err = badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer DB.Close()

	//Todo:
	//  /value
	//  /secret
	//  /stream

	rtr := mux.NewRouter()
	rtr.HandleFunc("/badger/{key}", badgerGet).Methods("GET")
	rtr.HandleFunc("/badger/{key}", badgerPut).Methods("POST","PUT")
	rtr.HandleFunc("/streamr/{prefix}", streamrGet).Methods("GET")
	rtr.HandleFunc("/secret/{key}", secretGet).Methods("GET")
	rtr.HandleFunc("/secret/{key}", secretPut).Methods("POST","PUT")
	http.Handle("/", rtr)
	//Todo: change to 8080
	log.Fatal(http.ListenAndServe(":8443",nil))
}
