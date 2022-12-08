package informer

import (
	"context"
	"log"

	k8scrd "github.com/Tsingshen/k8scrd/client"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

var (
	dynaCs = k8scrd.GetDynamicClient()
	hpcGvr = schema.GroupVersionResource{Group: "autoscaling.cloud.tencent.com", Version: "v1", Resource: "horizontalpodcronscalers"}
)

func ekletDeployAdd(cs *kubernetes.Clientset, lc *LocalConfig, d *appsv1.Deployment) error {
	log.Printf("deployment=%s/%s running ekletDeployAdd()", d.Namespace, d.Name)
	newDeploy := newEkletDeployment(d, lc)

	err := createUpdateDeploy(cs, newDeploy)
	if err != nil {
		return err
	}
	// add default hpc
	hpcObj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "autoscaling.cloud.tencent.com/v1",
			"kind":       "HorizontalPodCronscaler",
			"metadata": map[string]string{
				"name":      newDeploy.Name + "-hpc",
				"namespace": newDeploy.Namespace,
			},
			"spec": map[string]interface{}{
				"crons": []map[string]interface{}{
					{
						"name":       "scale-up",
						"schedule":   "1 1 1 1 1 1",
						"targetSize": 0,
					},
					{
						"name":       "scale-down",
						"schedule":   "10 1 1 1 1 1",
						"targetSize": 0,
					},
				},
				"scaleTarget": map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       newDeploy.Name,
					"namespace":  newDeploy.Namespace,
				},
			},
		},
	}

	_, err = dynaCs.Resource(hpcGvr).Namespace(newDeploy.Namespace).Create(context.Background(), &hpcObj, metav1.CreateOptions{
		FieldManager: "eklet-deployment-used",
	})
	if err != nil {
		return err
	}
	log.Printf("INFO: create hpc %s/%s success\n", newDeploy.Name, newDeploy.Namespace)

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
	log.Printf("INFO: delete deployment %s/%s success\n", delDeploy.Name, delDeploy.Namespace)

	err = dynaCs.Resource(hpcGvr).Namespace(delDeploy.Namespace).Delete(context.Background(), delDeploy.Name+"-hpc", metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	log.Printf("INFO: delete hpc %s/%s success\n", delDeploy.Name, delDeploy.Namespace)

	return nil
}
