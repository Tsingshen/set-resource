package informer

import (
	"context"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ekletDeployAdd(cs *kubernetes.Clientset, lc *LocalConfig, d *appsv1.Deployment) error {
	log.Printf("deployment=%s/%s running ekletDeployAdd()", d.Namespace, d.Name)
	newDeploy := newEkletDeployment(d, lc)

	err := createUpdateDeploy(cs, newDeploy)
	if err != nil {
		return err
	}
	return nil
}

func ekletDeployUpdate(cs *kubernetes.Clientset, lc *LocalConfig, newd *appsv1.Deployment) error {
	log.Printf("deployment=%s/%s running ekletDeployUpdate()", newd.Namespace, newd.Name)
	newEkletDeploy := newEkletDeployment(newd, lc)
	err := createUpdateDeploy(cs, newEkletDeploy)
	if err != nil {
		return err
	}
	return nil
}

func ekletDeployDelete(cs *kubernetes.Clientset, lc *LocalConfig, d *appsv1.Deployment) error {

	log.Printf("deployment=%s/%s running ekletDeployDelete()", d.Namespace, d.Name)
	delDeploy := newEkletDeployment(d, lc)

	err := cs.AppsV1().Deployments(delDeploy.Namespace).Delete(context.Background(), delDeploy.Name, metav1.DeleteOptions{})

	if err != nil {
		return err
	}

	return nil
}
