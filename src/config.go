package main

import (
	"context"
	"fmt"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/badger/pb"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"
	"github.com/rs/xid"
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
	CLI *client.Client
)


func createSecret(secretString string, secretName string) string {
	secretannotation := swarm.Annotations{Name: secretName, Labels: nil}
	secretdata := []byte(secretString)
	response, err := CLI.SecretCreate(context.Background(), swarm.SecretSpec{
		secretannotation, secretdata, nil, nil,
	})
	if err != nil {
		log.Print(err)
		return ""
	}
	return response.ID
}

func createService(serviceName string, imageName string) string {

	var serviceSpec swarm.ServiceSpec
	containerSpec := &swarm.ContainerSpec{Image: imageName}
	serviceSpec.TaskTemplate.ContainerSpec = containerSpec
	networkAttachmentConfig := swarm.NetworkAttachmentConfig{Target: getNetworkID("gaussnet")} //Todo gaussnet -> ??net
	serviceSpec.TaskTemplate.Networks = append(serviceSpec.TaskTemplate.Networks, networkAttachmentConfig)
	annotations := swarm.Annotations{Name: serviceName}
	serviceSpec.Annotations = annotations
	fileMode := os.FileMode(uint32(0))
	secretReferenceFileTarget := &swarm.SecretReferenceFileTarget{"/var/run/secrets/tPassword", "0", "0", fileMode}
	secret := &swarm.SecretReference{File: secretReferenceFileTarget, SecretID: getSecretID(strings.Split(getValue("tPassword"),"docker-secret:")[1]), SecretName: strings.Split(getValue("tPassword"),"docker-secret:")[1]}
	serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

	var serviceCreateOptions types.ServiceCreateOptions
	response, err := CLI.ServiceCreate(context.Background(), serviceSpec, serviceCreateOptions)
	if err != nil {
		log.Print(err)
		return ""
	}
	return response.ID
}

func deleteSecret(secretName string) {
	err := CLI.SecretRemove(context.Background(), getSecretID(secretName))
	if err != nil {
		log.Print(err)
	}
}

func deleteService(serviceName string) {
	err := CLI.ServiceRemove(context.Background(), getServiceID(serviceName))
	if err != nil {
		log.Print(err)
	}
}

func getSecretID(secretName string) string {
	secrets, err := CLI.SecretList(context.Background(), types.SecretListOptions{})
	if err != nil {
		log.Print(err)
		return ""
	}
	for _, secret := range secrets {
		if secret.Spec.Name == secretName {
			return secret.ID
		}
	}
	return ""
}

func getServiceID(secretName string) string {
	services, err := CLI.ServiceList(context.Background(), types.ServiceListOptions{})
	if err != nil {
		log.Print(err)
		return ""
	}
	for _, service := range services {
		if service.Spec.Name == secretName {
			return service.ID
		}
	}
	return ""
}

func getNetworkID(networkName string) string {
	networks, err := CLI.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		log.Print(err)
		return ""
	}
	for _, network := range networks {
		if network.Name == networkName {
			return network.ID
		}
	}
	return ""
}

func badgerGet(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	data := getValue(key)
	fmt.Fprintf(w,"%s",data)
	log.Printf("badger GET Key: %s Value: %s \n",key,data)
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

func badgerStream(w http.ResponseWriter, r *http.Request){
	prefix := mux.Vars(r)["prefix"]
	log.Printf("streamr GET prefix: %s", prefix)
	stream := DB.NewStream()

	stream.NumGo = 2
	stream.Prefix = []byte(prefix)
	stream.LogPrefix = "Badger.Streaming"

	// Send is called serially, while Stream.Orchestrate is running.
	stream.Send = func(list *pb.KVList) error {
		return proto.MarshalText(w, list) // Write to w.
	}

	// Run the stream
	if err := stream.Orchestrate(context.Background()); err != nil {
		log.Printf("error: %s", err)
	}
	// Done.
}

func secretPut(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil { panic(err) }
	key := mux.Vars(r)["key"]
  id := xid.New()
	createSecret(string(data),id.String())
  putValue(key,"docker-secret:"+id.String())
	fmt.Fprintf(w,"docker-secret:%s",id.String())
	log.Printf("secret PUT Badger Key: %s Docker Secret: %s \n",key,id.String())
  // ToDo: implement a metod for cleaning up old versions of secrets.
}

func startService(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if name == "query" {
		//Todo: change this to update if exists -or- create
		deleteService("query")
		createService("query", "gaussmeter/query")
		fmt.Fprintf(w, "ok")
	} else {
		fmt.Fprintf(w, "nothing to start...")
	}
	log.Printf("Starting %s",name)
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

func getValue(key string) string{
	txn := DB.NewTransaction(false)
	item, err := txn.Get([]byte(key))
	if err != nil {
		log.Print(err.Error() + " - " + key)
		return getDefault(key)
	}
	val, err := item.ValueCopy(nil)
	if err != nil {log.Fatal(err)}
	return string(val)
}

func getDefault(key string) string{
	txn := DB.NewTransaction(false)
	key = "default:"+key
	item, err := txn.Get([]byte(key))
	if err != nil {
		log.Print(err.Error() + " - " + key )
		return ""
	}
	val, err := item.ValueCopy(nil)
	if err != nil {log.Fatal(err)}
	return string(val)
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
				log.Print("Closing Docker Client")
				CLI.Close()
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

	opts := badger.DefaultOptions
	opts.Dir = "/tmp/badger"
	opts.ValueDir = "/tmp/badger"
	opts.ValueLogLoadingMode = options.FileIO
	opts.NumMemtables = 2
	opts.NumLevelZeroTables = 2
	opts.NumLevelZeroTablesStall = 4
	DB, err = badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer DB.Close()

	CLI, err = client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil {
		panic(err)
	}
	defer CLI.Close()

	putDefault("tHome","37.4919392,-121.9469367")
	putDefault("tHomeRadiusFt","100")
	putDefault("tWork","37.4919392,-121.9469367")
	putDefault("tWorkRadiusFt","100")
	putDefault("tChargeRangeFull", "270")
	putDefault("tChargeRangeMedium","100")
	putDefault("tChargeRangeLow","30")
	putDefault("eIHIP","fill")
	putDefault("eIHNP","fill")
	putDefault("eIHNPBCRM","fill")
	putDefault("eNH","rainbow")
	putDefault("cIHIP","0,0,0,255")
	putDefault("cIHNP","0,0,0,255")
	putDefault("cIHNPBCRM","0,0,0,255")
	putDefault("cNH","0,0,0,255")
	putDefault("tGetStateInterval","14400")
	putDefault("tSoftStateInterval","600")
	putDefault("tGetStateIntervalDriving","30")
	putDefault("tGetStateIntervalCharging","60")
	//tEmailAdr
	//tPassword

	rtr := mux.NewRouter()
	rtr.HandleFunc("/badger/{key}", badgerGet).Methods("GET")
	rtr.HandleFunc("/badger/{key}", badgerPut).Methods("PUT")
	rtr.HandleFunc("/badger/{key}", badgerPut).Methods("POST")
	rtr.HandleFunc("/streamr/{prefix}", badgerStream).Methods("GET")
	rtr.HandleFunc("/secret/{key}", badgerGet).Methods("GET")
	rtr.HandleFunc("/secret/{key}", secretPut).Methods("PUT")
	rtr.HandleFunc("/secret/{key}", secretPut).Methods("POST")
	rtr.HandleFunc("/start/{name}", startService).Methods("POST")
	rtr.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	http.Handle("/", rtr)

	log.Fatal(http.ListenAndServeTLS(":8443", "server.crt", "server.key", nil))
}
