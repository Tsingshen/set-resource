package informer

import (
	"fmt"
	"log"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func WatchDeploymentResource(cs *kubernetes.Clientset, lc *LocalConfig, enableSetResource, enableEkletDeployment bool, stopCh <-chan struct{}) error {
	informersFatory := informers.NewSharedInformerFactory(cs, time.Minute*10)
	deployInformer := informersFatory.Apps().V1().Deployments()
	watchNs := lc.IncludeNamespace

	deployInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			deploy := obj.(*appsv1.Deployment)

			if enableSetResource {
				if watchNs != nil {
					if checkSliceIncludeStr(watchNs, deploy.Namespace) {
						go func() {
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
						}()

					}

				}
			}

			if enableEkletDeployment {
				if checkInEkletDeployment(deploy, lc) {
					go func() {
						err := ekletDeployAdd(cs, lc, deploy)
						if err != nil {
							log.Printf("add eklet deployment error=%v\n", err)
						}
					}()
				}
			}
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			oldDeploy := oldObj.(*appsv1.Deployment)
			newDeploy := newObj.(*appsv1.Deployment)

			// have return, last run
			if enableSetResource {
				if watchNs != nil {
					if checkSliceIncludeStr(watchNs, oldDeploy.Namespace) {
						go func() {
							deployAnno := newDeploy.Annotations
							v, ok := deployAnno["not-reset-resources"]
							if ok {
								if v == "true" {
									// log.Printf("deployment %s/%s set anno not-reset-resource, skip updateFunc\n", newDeploy.Namespace, newDeploy.Name)
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
						}()
					}
				}
			}
			// no return, first run
			if enableEkletDeployment {
				if checkInEkletDeployment(newDeploy, lc) {
					if !reflect.DeepEqual(oldDeploy.Spec, newDeploy.Spec) {
						go func() {
							err := ekletDeployUpdate(cs, lc, newDeploy)
							if err != nil {
								log.Printf("update eklet deployment error=%v\n", err)
							}
						}()
					}
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			deploy := obj.(*appsv1.Deployment)
			if enableEkletDeployment {
				if checkInEkletDeployment(deploy, lc) {
					err := ekletDeployDelete(cs, lc, deploy)
					if err != nil {
						log.Printf("delete eklet deployment error=%v\n", err)
						return
					}
				}
			}
		},
	})

	// start informer
	informersFatory.Start(stopCh)
	informersFatory.WaitForCacheSync(stopCh)

	<-stopCh
	fmt.Println("deployment informer received stopped signal, exitting...")
	return nil
}
