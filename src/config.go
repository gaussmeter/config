package main

import (
  "context"
  "os"
  "github.com/docker/docker/api/types"
  "github.com/docker/docker/api/types/swarm"
  "html/template"
  "github.com/docker/docker/client"
  "log"
  "net/http"
)


type gauss struct {
  GaussUserName string
  GaussPassword string
  GaussHome string
}

func createSecret(secretString string, secretName string) (string) {
  cli, err :=  client.NewClientWithOpts(client.WithVersion("1.38"))
  if err != nil {panic(err)}
  secretannotation := swarm.Annotations{Name:secretName, Labels:nil}
  secretdata :=[]byte(secretString)
  response, err := cli.SecretCreate(context.Background(), swarm.SecretSpec{
    secretannotation, secretdata, nil,nil,
  })
  if err != nil {
    log.Print(err)
    return ""
  }
  return response.ID
}

func createService(serviceName string, imageName string) (string) {
  cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
  if err != nil {panic(err)}

  var serviceSpec swarm.ServiceSpec
  containerSpec := &swarm.ContainerSpec{Image: imageName}
  serviceSpec.TaskTemplate.ContainerSpec = containerSpec
  annotations := swarm.Annotations{Name:serviceName}
  serviceSpec.Annotations = annotations
  fileMode := os.FileMode(uint32(0))
  secretReferenceFileTarget := &swarm.SecretReferenceFileTarget{"/var/run/secrets/password","0","0",fileMode}
  secret := &swarm.SecretReference{File:secretReferenceFileTarget,SecretID:getSecretID("GaussPassword"),SecretName:"GaussPassword"}
  serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

  secretReferenceFileTarget = &swarm.SecretReferenceFileTarget{"/var/run/secrets/email","0","0",fileMode}
  secret = &swarm.SecretReference{File:secretReferenceFileTarget,SecretID:getSecretID("GaussUserName"),SecretName:"GaussUserName"}
  serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

  secretReferenceFileTarget = &swarm.SecretReferenceFileTarget{"/var/run/secrets/home","0","0",fileMode}
  secret = &swarm.SecretReference{File:secretReferenceFileTarget,SecretID:getSecretID("GaussHome"),SecretName:"GaussHome"}
  serviceSpec.TaskTemplate.ContainerSpec.Secrets = append(serviceSpec.TaskTemplate.ContainerSpec.Secrets, secret)

  var serviceCreateOptions types.ServiceCreateOptions
  response, err := cli.ServiceCreate(context.Background(),serviceSpec, serviceCreateOptions)
  if err != nil {
    log.Print(err)
    return ""
  }
  return response.ID
}

func deleteSecret(secretName string) {
  cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
  if err != nil { panic(err)}
  err = cli.SecretRemove(context.Background(), getSecretID(secretName) )
  if err != nil {
    log.Print(err)
  }
}

func deleteService(serviceName string){
  cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
  if err != nil { panic(err)}
  err = cli.ServiceRemove(context.Background(), getServiceID(serviceName))
  if err != nil {
    log.Print(err)
  }
}

func getSecretID(secretName string) (string) {
  cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
  if err != nil { panic(err)}
  secrets, err := cli.SecretList(context.Background(),types.SecretListOptions{})
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

func getServiceID(secretName string) (string) {
  cli, err := client.NewClientWithOpts(client.WithVersion("1.38"))
  if err != nil { panic(err)}
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


func gaussHandler(w http.ResponseWriter, r *http.Request) {
  if r.FormValue("submit") == "submit" {
    log.Print("submit")
    deleteService("query")
    deleteSecret("GaussUserName")
    deleteSecret("GaussPassword")
    deleteSecret("GaussHome")
    createSecret(r.FormValue("GaussUserName"),"GaussUserName")
    createSecret(r.FormValue("GaussPassword"),"GaussPassword")
    createSecret(r.FormValue("GaussPassword"),"GaussHome")
    createService("query","gaussmeter/query")
  }
  f := gauss{r.FormValue("GaussUserName"), r.FormValue("GaussPassword"), r.FormValue("GaussHome")}
  t, err := template.ParseFiles("gauss.html")
  if err != nil {
    log.Print(err)
  }
  err = t.Execute(w, f)
  if err != nil {
    log.Print(err)
  }
}

func main() {
  http.HandleFunc("/gauss", gaussHandler)
  log.Fatal(http.ListenAndServeTLS(":8443", "server.crt", "server.key", nil))
}
