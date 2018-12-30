package main

import (
	"context"
	"fmt"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/rs/xid"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

var (
	DB *badger.DB
	CLI *client.Client
)

type gauss struct {
	GaussUserName string
	GaussPassword string
	GaussHome     string
}

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
	networkAttachmentConfig := swarm.NetworkAttachmentConfig{Target: getNetworkID("gaussnet")}
	serviceSpec.TaskTemplate.Networks = append(serviceSpec.TaskTemplate.Networks, networkAttachmentConfig)
	annotations := swarm.Annotations{Name: serviceName}
	serviceSpec.Annotations = annotations
	fileMode := os.FileMode(uint32(0))
	secretReferenceFileTarget := &swarm.SecretReferenceFileTarget{"/var/run/secrets/password", "0", "0", fileMode}
	secret := &swarm.SecretReference{File: secretReferenceFileTarget, SecretID: getSecretID(strings.Split(getValue("GaussPassword"),"docker-secret:")[1]), SecretName: strings.Split(getValue("GaussPassword"),"docker-secret:")[1]}
	serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

	secretReferenceFileTarget = &swarm.SecretReferenceFileTarget{"/var/run/secrets/email", "0", "0", fileMode}
	secret = &swarm.SecretReference{File: secretReferenceFileTarget, SecretID: getSecretID("GaussUserName"), SecretName: "GaussUserName"}
	serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

	secretReferenceFileTarget = &swarm.SecretReferenceFileTarget{"/var/run/secrets/home", "0", "0", fileMode}
	secret = &swarm.SecretReference{File: secretReferenceFileTarget, SecretID: getSecretID("GaussHome"), SecretName: "GaussHome"}
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
	log.Printf("GET Key: %s Value: %s \n",key,data)
}

func badgerPut(w http.ResponseWriter, r *http.Request) {
	//ToDo: implement TTL https://github.com/dgraph-io/badger#setting-time-to-livettl-and-user-metadata-on-keys
	// ?? use header TTL=Value ??
	data, err := ioutil.ReadAll(r.Body)
	if err != nil { panic (err) }
	key := mux.Vars(r)["key"]
	putValue(key,string(data))
	fmt.Fprintf(w,"ok")
	log.Printf("PUT Key: %s Value: %s \n",key,data)
}

func secretPut(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil { panic(err) }
	key := mux.Vars(r)["key"]
  id := xid.New()
	createSecret(string(data),id.String())
  putValue(key,"docker-secret:"+id.String())
	fmt.Fprintf(w,"docker-secret:%s",id.String())
	log.Printf("PUT Badger Key: %s Docker Secret: %s \n",key,id.String())
  // ToDo: implement a metod for cleaning up old versions of secrets.
}

func startService(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	if name == "query" {
		deleteService("query")
		deleteSecret("GaussUserName")
		deleteSecret("GaussHome")
		createSecret(getValue("GaussUserName"), "GaussUserName")
		createSecret(getValue("GaussHome"), "GaussHome")
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
		log.Print(err)
		return ""
	}
	val, err := item.ValueCopy(nil)
	if err != nil {log.Fatal(err)}
	return string(val)
}


func main() {
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

	rtr := mux.NewRouter()
	//rtr.HandleFunc("/gauss", gaussHandler)
	rtr.HandleFunc("/badger/{key}", badgerGet).Methods("GET")
	rtr.HandleFunc("/badger/{key}", badgerPut).Methods("PUT")
	rtr.HandleFunc("/badger/{key}", badgerPut).Methods("POST")
	rtr.HandleFunc("/secret/{key}", badgerGet).Methods("GET")
	rtr.HandleFunc("/secret/{key}", secretPut).Methods("PUT")
	rtr.HandleFunc("/secret/{key}", secretPut).Methods("POST")
	rtr.HandleFunc("/start/{name}", startService).Methods("POST")
	rtr.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
	http.Handle("/", rtr)

	log.Fatal(http.ListenAndServeTLS(":8443", "server.crt", "server.key", nil))
}

// ToDo: implement https://github.com/dgraph-io/badger#garbage-collection
// ToDo: add Gaussmeter fav icon
