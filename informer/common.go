package informer

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

var (
	CONTAINER_APP_NAME        = "app"
	UpdateDeployRateMax int32 = 20
)

type LocalConfig struct {
	Resource            ConfigResource `yaml:"resource,omitempty"`
	IncludeNamespace    []string       `yaml:"includeNamespace,omitempty"`
	UpdateDeployRateMax int32          `yaml:"updateDeployRateMax,omitempty"`
	EkletDeployment     struct {
		Deployment   []string          `yaml:"deloyment"`
		NodeSelector map[string]string `yaml:"nodeSelector"`
		Prefix       string            `yaml:"prefix"`
	} `yaml:"ekletDeployment,omitempty"`
}

type ConfigResource struct {
	Requests CpuAndMem `yaml:"requests"`
	Limits   CpuAndMem `yaml:"limits"`
}

type CpuAndMem struct {
	Cpu    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
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

func waitDeploymentUpdate(cs *kubernetes.Clientset, d *appsv1.Deployment) error {
	if d == nil {
		return fmt.Errorf("can not watch nil deployment update")
	}

	if d.Spec.Replicas == pointer.Int32(0) {
		return nil
	}

	for i := 0; i < 20; i++ {
		time.Sleep(3 * time.Second)
		if d.Status.AvailableReplicas == d.Status.Replicas && d.Status.ReadyReplicas == d.Status.Replicas &&
			d.Status.UpdatedReplicas == d.Status.Replicas {
			break
		}
		if i == 19 {
			return fmt.Errorf("wait for 1 minutes deployment = %s/%s not update success, skip watch update", d.Namespace, d.Name)
		}
	}
	return nil
}

func checkInEkletDeployment(deploy *appsv1.Deployment, lc *LocalConfig) bool {
	if strings.HasPrefix(deploy.Name, lc.EkletDeployment.Prefix) {
		return false
	}
	m := make(map[string]string, 200)
	for _, v := range lc.EkletDeployment.Deployment {
		s := strings.Split(v, "/")
		m[s[0]] = s[1]
	}

	return m[deploy.Namespace] == deploy.Name
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
	// todo
	lc.UpdateDeployRateMax = UpdateDeployRateMax
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
			deploy, err := cs.AppsV1().Deployments(dCopy.Namespace).Update(context.Background(), dCopy, metav1.UpdateOptions{
				FieldManager: "set-resource-client",
			})
			log.Printf("update deployment with resource limit: %s,%s, request: %s,%s, %s/%s\n",
				lc.Resource.Requests.Cpu, lc.Resource.Requests.Memory, lc.Resource.Limits.Cpu, lc.Resource.Limits.Memory,
				dCopy.Namespace, dCopy.Name)
			if err != nil {
				return err
			}

			if err := waitDeploymentUpdate(cs, deploy); err != nil {
				return err
			}
			break
		}

	}

	return nil
}

func findDeployment(cs *kubernetes.Clientset, name, ns string) (int32, bool) {
	d, err := cs.AppsV1().Deployments(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("findDeployment=%s/%s error=%v\n", ns, name, err)
		return -1, false
	}
	return *d.Spec.Replicas, true

}

func newEkletDeployment(d *appsv1.Deployment, lc *LocalConfig) *appsv1.Deployment {
	dCopy := d.DeepCopy()

	// change name
	dCopy.Name = lc.EkletDeployment.Prefix + d.Name
	// change replicas == zero
	dCopy.Spec.Replicas = pointer.Int32(0)
	// add nodeSelector to eklet node
	if dCopy.Spec.Template.Spec.NodeSelector == nil {
		dCopy.Spec.Template.Spec.NodeSelector = make(map[string]string, 1)
	}
	dCopy.Spec.Template.Spec.NodeSelector = lc.EkletDeployment.NodeSelector
	//add ds-inject anno
	if dCopy.Spec.Template.Annotations == nil {
		dCopy.Spec.Template.Annotations = make(map[string]string, 1)
	}
	dCopy.Spec.Template.Annotations["eks.tke.cloud.tencent.com/ds-injection"] = "true"
	// add eklet node toreration
	dCopy.Spec.Template.Spec.Tolerations = append(dCopy.Spec.Template.Spec.Tolerations, corev1.Toleration{
		Key:      "eks.tke.cloud.tencent.com/eklet",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	})
	// add not-reset-resources anno
	if dCopy.Annotations == nil {
		dCopy.Annotations = make(map[string]string, 1)
	}
	dCopy.Annotations["not-reset-resources"] = "true"

	// remove some no use info
	delete(dCopy.ObjectMeta.Annotations, "deployment.kubernetes.io/revision")
	delete(dCopy.ObjectMeta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	dCopy.ResourceVersion = ""
	dCopy.UID = ""
	dCopy.Status = appsv1.DeploymentStatus{}

	return dCopy
}

func createUpdateDeploy(cs *kubernetes.Clientset, deploy *appsv1.Deployment) error {
	// if found update or create
	if replicas, ok := findDeployment(cs, deploy.Name, deploy.Namespace); ok {
		log.Printf("INFO: eklet deployment=%s/%s found, update...\n", deploy.Namespace, deploy.Name)
		deploy.Spec.Replicas = &replicas
		_, err := cs.AppsV1().Deployments(deploy.Namespace).Update(context.Background(), deploy, metav1.UpdateOptions{
			FieldManager: "eklet-informer-update",
		})
		if err != nil {
			return err
		}
		log.Printf("INFO: eklet deployment=%s/%s update success\n", deploy.Namespace, deploy.Name)
		return nil
	}

	log.Printf("INFO: eklet deployment=%s/%s not found, create\n", deploy.Namespace, deploy.Name)
	_, err := cs.AppsV1().Deployments(deploy.Namespace).Create(context.Background(), deploy, metav1.CreateOptions{
		FieldManager: "eklet-informer-create",
	})
	if err != nil {
		return err
	}
	log.Printf("INFO: eklet deployment=%s/%s crete success\n", deploy.Namespace, deploy.Name)
	return nil

}
