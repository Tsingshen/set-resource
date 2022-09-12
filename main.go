package main

import (
	"context"
	"log"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/Tsingshen/k8scrd/client"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var (
	CONTAINER_APP_NAME        = "app"
	updateDeployRateMax int32 = 20
)

type LocalConfig struct {
	Resource            configResource `yaml:"resource"`
	IncludeNamespace    []string       `yaml:"includeNamespace"`
	UpdateDeployRateMax int32          `yaml:"updateDeployRateMax"`
}

type configResource struct {
	Requests CpuAndMem `yaml:"requests"`
	Limits   CpuAndMem `yaml:"limits"`
}

type CpuAndMem struct {
	Cpu    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

func main() {
	var lc = &LocalConfig{}
	cs := client.GetClient()
	ch := make(chan struct{})
	// set viper

	viper.SetConfigName("config.yaml")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		log.Panicf("fatal error config file: %v\n", err)
	}

	if err := viper.Unmarshal(lc); err != nil {
		log.Panicf("unmarshal config err: %v\n", err)
	}

	go func() {
		log.Printf("Set-resource Request: cpu=%s,mem=%s, Limit: cpu=%s,mem=%s\n", lc.Resource.Requests.Cpu, lc.Resource.Requests.Memory, lc.Resource.Limits.Cpu, lc.Resource.Limits.Memory)
		err = watchDeploymentResource(cs, lc, ch)
		if err != nil {
			panic(err)
		}

	}()

	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		if in.Op&fsnotify.Write == fsnotify.Write {
			if err := viper.Unmarshal(&lc); err != nil {
				panic(err)
			}
			ch <- struct{}{}
			go func() {
				log.Printf("Set-resource Request: cpu=%s,mem=%s, Limit: cpu=%s,mem=%s\n", lc.Resource.Requests.Cpu, lc.Resource.Requests.Memory, lc.Resource.Limits.Cpu, lc.Resource.Limits.Memory)
				err = watchDeploymentResource(cs, lc, ch)
				if err != nil {
					panic(err)
				}
			}()
		}
	})

	select {}

}

func watchDeploymentResource(cs *kubernetes.Clientset, lc *LocalConfig, ch chan struct{}) error {
	informersFatory := informers.NewSharedInformerFactory(cs, time.Minute*1)
	deployInformer := informersFatory.Apps().V1().Deployments()
	watchNs := lc.IncludeNamespace

	deployInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if watchNs != nil {
				deploy := obj.(*appsv1.Deployment)
				if checkSliceIncludeStr(watchNs, deploy.Namespace) {
					deployAnno := deploy.Annotations
					v, ok := deployAnno["not-reset-resources"]
					if ok {
						if v == "true" {
							log.Printf("deployment %s/%s set anno not-reset-resource, skip addFunc\n", deploy.Namespace, deploy.Name)
							return
						}
					}
					deployResource := getDeployResource(cs, deploy)
					if !reflect.DeepEqual(deployResource, corev1.ResourceRequirements{}) {
						if !checkResourceEqual(lc, &deployResource) {
							err := lc.updateDeployResource(cs, deploy)
							if err != nil {
								log.Printf("Error: update deployment %s/%s resource err: %v\n", deploy.Namespace, deploy.Name, err)
								return
							}
						}
					} else {
						err := lc.updateDeployResource(cs, deploy)
						if err != nil {
							log.Printf("Error: update deployment %s/%s resource err: %v\n", deploy.Namespace, deploy.Name, err)
							return
						}
					}

				}

			}
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			if watchNs != nil {
				oldDeploy := oldObj.(*appsv1.Deployment)
				newDeploy := newObj.(*appsv1.Deployment)

				if checkSliceIncludeStr(watchNs, oldDeploy.Namespace) {
					deployAnno := newDeploy.Annotations
					v, ok := deployAnno["not-reset-resources"]
					if ok {
						if v == "true" {
							log.Printf("depoyment %s/%s set anno not-reset-resource, skip updateFunc\n", newDeploy.Namespace, newDeploy.Name)
							return
						}
					}

					newDeployResource := getDeployResource(cs, newDeploy)
					oldDeployResource := getDeployResource(cs, oldDeploy)

					if reflect.DeepEqual(newDeployResource, oldDeployResource) {
						return
					}

					if reflect.DeepEqual(newDeployResource, corev1.ResourceRequirements{}) {
						err := lc.updateDeployResource(cs, newDeploy)
						if err != nil {
							log.Printf("Error: UpdateFunc update deployemnt %s/%s err: %v\n", newDeploy.Namespace, newDeploy.Name, err)
							return
						}
						return
					}

					if !checkResourceEqual(lc, &newDeployResource) {
						err := lc.updateDeployResource(cs, newDeploy)
						if err != nil {
							log.Printf("Error: UpdateFunc update deployemnt %s/%s err: %v\n", newDeploy.Namespace, newDeploy.Name, err)
							return
						}
					}
				}
			}
		},
	})
	stopCh := make(chan struct{})
	defer close(stopCh)

	// start informer
	informersFatory.Start(stopCh)
	informersFatory.WaitForCacheSync(stopCh)

	stopCh <- <-ch

	return nil

}

func checkResourceEqual(lc *LocalConfig, r2 *corev1.ResourceRequirements) bool {
	r1 := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(lc.Resource.Limits.Cpu),
			corev1.ResourceMemory: resource.MustParse(lc.Resource.Limits.Memory),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(lc.Resource.Requests.Cpu),
			corev1.ResourceMemory: resource.MustParse(lc.Resource.Requests.Memory),
		},
	}

	if r1 == nil && r2 != nil || r1 != nil && r2 == nil {
		return false
	}

	if r1 == nil && r2 == nil {
		return true
	}

	return r2.Limits.Cpu().Equal(*r1.Limits.Cpu()) &&
		r2.Limits.Memory().Equal(*r1.Limits.Memory()) &&
		r2.Requests.Cpu().Equal(*r1.Requests.Cpu()) &&
		r2.Requests.Memory().Equal(*r1.Requests.Memory())

}

func getDeployResource(cs *kubernetes.Clientset, d *appsv1.Deployment) corev1.ResourceRequirements {
	c := d.Spec.Template.Spec.Containers

	for _, v := range c {
		if v.Name == CONTAINER_APP_NAME {
			if !reflect.DeepEqual(v.Resources, corev1.ResourceRequirements{}) {
				return v.Resources
			}
		}
	}

	return corev1.ResourceRequirements{}
}

func (lc *LocalConfig) updateDeployResource(cs *kubernetes.Clientset, d *appsv1.Deployment) error {

	// for the sake of loop run nothing
	time.Sleep(time.Millisecond * 500)

	lc.UpdateDeployRateMax = updateDeployRateMax
	leftToken := atomic.AddInt32(&lc.UpdateDeployRateMax, -1)
	defer atomic.AddInt32(&lc.UpdateDeployRateMax, 1)
	if leftToken > 0 {
		dCopy := d.DeepCopy()
		c := dCopy.Spec.Template.Spec.Containers

		for k, v := range c {
			if v.Name == CONTAINER_APP_NAME {
				c[k].Resources = corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(lc.Resource.Limits.Cpu),
						corev1.ResourceMemory: resource.MustParse(lc.Resource.Limits.Memory),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(lc.Resource.Requests.Cpu),
						corev1.ResourceMemory: resource.MustParse(lc.Resource.Requests.Memory),
					},
				}
				dCopy.ResourceVersion = ""
				_, err := cs.AppsV1().Deployments(dCopy.Namespace).Update(context.Background(), dCopy, metav1.UpdateOptions{
					FieldManager: "set-resource-client",
				})
				log.Printf("update deployment %s/%s with resource limit: %s,%s, request: %s,%s\n",
					dCopy.Namespace, dCopy.Name, lc.Resource.Requests.Cpu, lc.Resource.Requests.Memory, lc.Resource.Limits.Cpu, lc.Resource.Limits.Memory)
				if err != nil {
					return err
				}
				break
			}
		}

	}

	return nil
}

func checkSliceIncludeStr(s []string, str string) bool {
	if s == nil || str == "" {
		return false
	}

	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
