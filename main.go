package main

import (
	"context"
	"log"
	"os"
	"reflect"
	"regexp"
	"time"

	"github.com/Tsingshen/k8scrd/client"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var (
	CONTAINER_APP_NAME = "app"
)

type Resource struct {
	CpuRequest string
	MemRequest string
	CpuLimit   string
	MemLimit   string
}

func main() {
	cs := client.GetClient()

	r := &Resource{
		CpuRequest: "10m",
		MemRequest: "56Mi",
		CpuLimit:   "2000m",
		MemLimit:   "2048Mi",
	}
	log.Printf("Set-resource Request: cpu=%s,mem=%s, Limit: cpu=%s,mem=%s\n", r.CpuRequest, r.MemRequest, r.CpuLimit, r.MemLimit)

	err := watchDeploymentResource(cs, r)
	if err != nil {
		panic(err)
	}

}

func watchDeploymentResource(cs *kubernetes.Clientset, r *Resource) error {
	informersFatory := informers.NewSharedInformerFactory(cs, time.Minute*2)

	deployInformer := informersFatory.Apps().V1().Deployments()
	watchNs := os.Getenv("WATCH_NS")

	nsReg := regexp.MustCompile(`shencq`)
	if watchNs != "" {
		nsReg = regexp.MustCompile(watchNs)
	}

	deployInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			deploy := obj.(*appsv1.Deployment)
			if nsReg.MatchString(deploy.Namespace) {
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
					if !checkResourceEqual(r, &deployResource) {
						err := r.updateDeployResource(cs, deploy)
						if err != nil {
							log.Printf("Error: update deployment %s/%s resource err: %v\n", deploy.Namespace, deploy.Name, err)
							return
						}
					}
				} else {
					err := r.updateDeployResource(cs, deploy)
					if err != nil {
						log.Printf("Error: update deployment %s/%s resource err: %v\n", deploy.Namespace, deploy.Name, err)
						return
					}
				}

			}

		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldDeploy := oldObj.(*appsv1.Deployment)
			newDeploy := newObj.(*appsv1.Deployment)

			if nsReg.MatchString(oldDeploy.Namespace) {
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
					err := r.updateDeployResource(cs, newDeploy)
					if err != nil {
						log.Printf("Error: UpdateFunc update deployemnt %s/%s err: %v\n", newDeploy.Namespace, newDeploy.Name, err)
						return
					}
					return
				}

				if !checkResourceEqual(r, &newDeployResource) {
					err := r.updateDeployResource(cs, newDeploy)
					if err != nil {
						log.Printf("Error: UpdateFunc update deployemnt %s/%s err: %v\n", newDeploy.Namespace, newDeploy.Name, err)
						return
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

	<-stopCh

	return nil

}

func checkResourceEqual(resource1 *Resource, resource2 *corev1.ResourceRequirements) bool {
	r1 := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(resource1.CpuLimit),
			corev1.ResourceMemory: resource.MustParse(resource1.MemLimit),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(resource1.CpuRequest),
			corev1.ResourceMemory: resource.MustParse(resource1.MemRequest),
		},
	}

	r2 := resource2

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

func (r *Resource) updateDeployResource(cs *kubernetes.Clientset, d *appsv1.Deployment) error {

	// for the sake of loop run nothing
	time.Sleep(time.Millisecond * 500)

	dCopy := d.DeepCopy()
	c := dCopy.Spec.Template.Spec.Containers

	for k, v := range c {
		if v.Name == CONTAINER_APP_NAME {
			c[k].Resources = corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(r.CpuLimit),
					corev1.ResourceMemory: resource.MustParse(r.MemLimit),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(r.CpuRequest),
					corev1.ResourceMemory: resource.MustParse(r.MemRequest),
				},
			}
			_, err := cs.AppsV1().Deployments(dCopy.Namespace).Update(context.Background(), dCopy, metav1.UpdateOptions{
				FieldManager: "set-resource-client",
			})
			log.Printf("update deployment %s/%s with default resource which you set\n", dCopy.Namespace, dCopy.Name)
			if err != nil {
				return err
			}
			break
		}
	}

	return nil
}
