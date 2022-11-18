package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"set-resource/informer"

	k8scrdClient "github.com/Tsingshen/k8scrd/client"
	"github.com/spf13/viper"
)

func main() {

	enableSetResource := flag.Bool("set-resource", false, "start set resources")
	enableEkletDeployment := flag.Bool("eklet-deployment", false, "start set resources")
	flag.Parse()

	lc := &informer.LocalConfig{}
	cs := k8scrdClient.GetClient()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// set viper config
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	v.SetConfigName("config.yaml")
	v.SetConfigType("yaml")
	// viper.AddConfigPath("/app/config")
	v.AddConfigPath("./config")
	err := v.ReadInConfig()
	if err != nil {
		log.Panicf("fatal error config file: %v\n", err)
	}

	if err := v.Unmarshal(lc); err != nil {
		log.Panicf("unmarshal config err: %v\n", err)
	}

	// run informer
	if *enableSetResource {
		log.Printf("set-resource is running, resources Request: cpu=%s,mem=%s, Limit: cpu=%s,mem=%s\n", lc.Resource.Requests.Cpu, lc.Resource.Requests.Memory, lc.Resource.Limits.Cpu, lc.Resource.Limits.Memory)
	}
	if *enableEkletDeployment {
		log.Printf("eklet-deployment is running, ekletDeployment config=%#v\n", lc.EkletDeployment)
	}
	err = informer.WatchDeploymentResource(cs, lc, *enableSetResource, *enableEkletDeployment, ctx.Done())
	if err != nil {
		panic(err)
	}
}
