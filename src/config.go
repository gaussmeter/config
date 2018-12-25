package main

import (
	"context"
	"github.com/dgraph-io/badger"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"html/template"
	"log"
	"net/http"
	"os"
)

var (
	DB *badger.DB
)

type gauss struct {
	GaussUserName string
	GaussPassword string
	GaussHome     string
}

func createSecret(secretString string, secretName string) string {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil {
		panic(err)
	}
	secretannotation := swarm.Annotations{Name: secretName, Labels: nil}
	secretdata := []byte(secretString)
	response, err := cli.SecretCreate(context.Background(), swarm.SecretSpec{
		secretannotation, secretdata, nil, nil,
	})
	if err != nil {
		log.Print(err)
		return ""
	}
	return response.ID
}

func createService(serviceName string, imageName string) string {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil {
		panic(err)
	}

	var serviceSpec swarm.ServiceSpec
	containerSpec := &swarm.ContainerSpec{Image: imageName}
	serviceSpec.TaskTemplate.ContainerSpec = containerSpec
	networkAttachmentConfig := swarm.NetworkAttachmentConfig{Target: getNetworkID("gaussnet")}
	serviceSpec.TaskTemplate.Networks = append(serviceSpec.TaskTemplate.Networks, networkAttachmentConfig)
	annotations := swarm.Annotations{Name: serviceName}
	serviceSpec.Annotations = annotations
	fileMode := os.FileMode(uint32(0))
	secretReferenceFileTarget := &swarm.SecretReferenceFileTarget{"/var/run/secrets/password", "0", "0", fileMode}
	secret := &swarm.SecretReference{File: secretReferenceFileTarget, SecretID: getSecretID("GaussPassword"), SecretName: "GaussPassword"}
	serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

	secretReferenceFileTarget = &swarm.SecretReferenceFileTarget{"/var/run/secrets/email", "0", "0", fileMode}
	secret = &swarm.SecretReference{File: secretReferenceFileTarget, SecretID: getSecretID("GaussUserName"), SecretName: "GaussUserName"}
	serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

	secretReferenceFileTarget = &swarm.SecretReferenceFileTarget{"/var/run/secrets/home", "0", "0", fileMode}
	secret = &swarm.SecretReference{File: secretReferenceFileTarget, SecretID: getSecretID("GaussHome"), SecretName: "GaussHome"}
	serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

	var serviceCreateOptions types.ServiceCreateOptions
	response, err := cli.ServiceCreate(context.Background(), serviceSpec, serviceCreateOptions)
	if err != nil {
		log.Print(err)
		return ""
	}
	return response.ID
}

func deleteSecret(secretName string) {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil {
		panic(err)
	}
	err = cli.SecretRemove(context.Background(), getSecretID(secretName))
	if err != nil {
		log.Print(err)
	}
}

func deleteService(serviceName string) {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil {
		panic(err)
	}
	err = cli.ServiceRemove(context.Background(), getServiceID(serviceName))
	if err != nil {
		log.Print(err)
	}
}

func getSecretID(secretName string) string {
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil {
		panic(err)
	}
	secrets, err := cli.SecretList(context.Background(), types.SecretListOptions{})
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
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil {
		panic(err)
	}
	services, err := cli.ServiceList(context.Background(), types.ServiceListOptions{})
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
	cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	if err != nil{
		panic(err)
	}
	networks, err := cli.NetworkList(context.Background(), types.NetworkListOptions{})
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

func gaussHandler(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("submit") == "submit" {
		log.Print("submit")
		deleteService("query")
		deleteSecret("GaussUserName")
		deleteSecret("GaussPassword")
		deleteSecret("GaussHome")
		createSecret(r.FormValue("GaussUserName"), "GaussUserName")
		createSecret(r.FormValue("GaussPassword"), "GaussPassword")
		createSecret(r.FormValue("GaussHome"), "GaussHome")
		createService("query", "gaussmeter/query")
		putValue("GaussUserName",r.FormValue("GaussUserName"))
		putValue("GaussHome",r.FormValue("GaussHome"))
	}
	f := gauss{getValue("GaussUserName"), "nope!", getValue("GaussHome")}
	t, err := template.ParseFiles("gauss.html")
	if err != nil {
		log.Print(err)
	}
	err = t.Execute(w, f)
	if err != nil {
		log.Print(err)
	}
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
	opts := badger.DefaultOptions
	opts.Dir = "/tmp/badger"
	opts.ValueDir = "/tmp/badger"
	var err error
	DB, err = badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer DB.Close()

	log.Print(getNetworkID("gaussnet"))
	http.HandleFunc("/gauss", gaussHandler)
	log.Fatal(http.ListenAndServeTLS(":8443", "server.crt", "server.key", nil))
}
