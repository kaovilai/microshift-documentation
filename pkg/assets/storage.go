package assets

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	scassets "github.com/openshift/microshift/pkg/assets/storage"
	"github.com/openshift/microshift/pkg/constant"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	scv1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	scclientv1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

var (
	scScheme = runtime.NewScheme()
	scCodecs = serializer.NewCodecFactory(scScheme)
)

func init() {
	if err := scv1.AddToScheme(scScheme); err != nil {
		panic(err)
	}
}

func scClient() *scclientv1.StorageV1Client {
	restConfig, err := clientcmd.BuildConfigFromFlags("", constant.AdminKubeconfigPath)
	if err != nil {
		panic(err)
	}

	return scclientv1.NewForConfigOrDie(rest.AddUserAgent(restConfig, "sc-agent"))
}

type scApplier struct {
	Client *scclientv1.StorageV1Client
	sc     *scv1.StorageClass
}

func (s *scApplier) Reader(objBytes []byte, render RenderFunc) {
	var err error
	if render != nil {
		objBytes, err = render(objBytes)
		if err != nil {
			panic(err)
		}
	}
	obj, err := runtime.Decode(scCodecs.UniversalDecoder(scv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	s.sc = obj.(*scv1.StorageClass)
}
func (s *scApplier) Applier() error {
	_, err := s.Client.StorageClasses().Get(context.TODO(), s.sc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := s.Client.StorageClasses().Create(context.TODO(), s.sc, metav1.CreateOptions{})
		return err
	}
	return nil
}

func applySCs(scs []string, applier readerApplier, render RenderFunc) error {
	lock.Lock()
	defer lock.Unlock()

	for _, sc := range scs {
		logrus.Infof("applying sc %s", sc)
		objBytes, err := scassets.Asset(sc)
		if err != nil {
			return fmt.Errorf("error getting asset %s: %v", sc, err)
		}
		applier.Reader(objBytes, render)
		if err := applier.Applier(); err != nil {
			logrus.Warningf("failed to apply sc api %s: %v", sc, err)
			return err
		}
	}

	return nil
}

func ApplyStorageClasses(scs []string, render RenderFunc) error {
	sc := &scApplier{}
	sc.Client = scClient()
	return applySCs(scs, sc, render)
}
