/*
Copyright 2018 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	_ "net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	
	//gcpuser "cloud.google.com/go/iam"
	gcs "cloud.google.com/go/storage"

	"github.com/golang/glog"
	"github.com/yard-turkey/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	libbkt "github.com/yard-turkey/lib-bucket-provisioner/pkg/provisioner"
	apibkt "github.com/yard-turkey/lib-bucket-provisioner/pkg/provisioner/api"

	storageV1 "k8s.io/api/storage/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultRegion    = "us-east4"
	defaultProject   = "openshift-gce-devel"
	httpPort         = 80
	httpsPort        = 443
	provisionerName  = "gcs.io/bucket"
	regionInsert     = "<REGION>"
	s3Hostname       = "s3-" + regionInsert + ".amazonaws.com"
	s3BucketArn      = "arn:aws:s3:::%s"
	policyArn        = "arn:aws:iam::%s:policy/%s"
	createBucketUser = false
	obStateARN	     = "ARN"
	obStateUser      = "UserName"
	maxBucketLen     = 58
	maxUserLen       = 63
	genUserLen       = 5
)

var (
	kubeconfig string
	masterURL  string
)

type gcsProvisioner struct {
	bucketName string
	region     string
	// project id
	projectId  string
	gcsClient  *gcs.Client
	//kube client
	clientset  *kubernetes.Clientset
	// access keys for aws acct for the bucket *owner*
	bktOwnerAccessId  string
	bktOwnerSecretKey string
	bktCreateUser     string
	bktUserName       string
	bktUserAccessId   string
	bktUserSecretKey  string
	bktUserAccountId  string
	bktUserPolicyArn  string
}

func NewGcsProvisioner(cfg *restclient.Config, s3Provisioner gcsProvisioner) (*libbkt.Provisioner, error) {

	const all_namespaces = ""
	return libbkt.NewProvisioner(cfg, provisionerName, s3Provisioner, all_namespaces)
}


// Return the OB struct with minimal fields filled in.
func (p *gcsProvisioner) rtnObjectBkt(bktName string) *v1alpha1.ObjectBucket {

	host := strings.Replace(s3Hostname, regionInsert, p.region, 1)
	conn := &v1alpha1.Connection{
		Endpoint: &v1alpha1.Endpoint{
			BucketHost: host,
			BucketPort: httpsPort,
			BucketName: bktName,
			Region:     p.region,
			SSL:        true,
		},
		Authentication: &v1alpha1.Authentication{
			AccessKeys: &v1alpha1.AccessKeys{
				AccessKeyID:     p.bktUserAccessId,
				SecretAccessKey: p.bktUserSecretKey,
			},
		},
		AdditionalState: map[string]string{
			obStateARN:  p.bktUserPolicyArn,
			obStateUser: p.bktUserName,
		},
	}

	return &v1alpha1.ObjectBucket{
		Spec: v1alpha1.ObjectBucketSpec{
			Connection: conn,
		},
	}
}

func (p *gcsProvisioner) createBucket(bktName string) error {

	// Give the bucket a unique name.
	ctx := context.Background()
	err := p.gcsClient.Bucket(bktName).Create(ctx, p.projectId, nil)
	if err != nil {
		return err
	}
	glog.Infof("Bucket %s successfully created", bktName)

	return nil
}

// Create the GCS Client
func (p *gcsProvisioner) setClientFromStorageClass(sc *storageV1.StorageClass) error {

	region := getRegion(sc)
	if region == "" {
		glog.Infof("region is empty in storage class %q, default region %q used", sc.Name, defaultRegion)
		region = defaultRegion
	}
	project := getProject(sc)
	if region == "" {
		glog.Infof("region is empty in storage class %q, default region %q used", sc.Name, defaultRegion)
		region = defaultProject
	}
	p.projectId = project;
	p.region = region

	// If running locally (with already configured authorization client)
	// or running directly on GCP - we should not have to authenticate
	// but we would need to figure out how to handle say running in AWS and
	// trying to create a bucket in GCP, if that would ever need to happen?
	ctx := context.Background()
	client, err := gcs.NewClient(ctx)
	if err != nil {
		glog.Fatal(err)
		return err
	}
	p.gcsClient = client


	return nil
}

// initializeCreateOrGrant sets common provisioner receiver fields and
// the services and sessions needed to provision.
func (p *gcsProvisioner) initializeCreateOrGrant(options *apibkt.BucketOptions) error {
	glog.V(2).Infof("initializing and setting CreateOrGrant services")
	// set the bucket name
	p.bucketName = options.BucketName
	// get the OBC and its storage class
	obc := options.ObjectBucketClaim
	scName := options.ObjectBucketClaim.Spec.StorageClassName
	sc, err := p.getClassByNameForBucket(scName)
	if err != nil {
		glog.Errorf("failed to get storage class for OBC \"%s/%s\": %v", obc.Namespace, obc.Name, err)
		return err
	}

	err = p.setClientFromStorageClass(sc)
	if err != nil {
		return fmt.Errorf("error creating AWS session: %v", err)
	}

	return nil
}

// Provision creates an GCS bucket and returns a connection info
// representing the bucket's endpoint and user access credentials.
func (p gcsProvisioner) Provision(options *apibkt.BucketOptions) (*v1alpha1.ObjectBucket, error) {

	// initialize and set the AWS services and commonly used variables
	err := p.initializeCreateOrGrant(options)
	if err != nil {
		return nil, err
	}

	// create the bucket
	glog.Infof("Creating bucket %q", p.bucketName)
	err = p.createBucket(p.bucketName)
	if err != nil {
		err = fmt.Errorf("error creating bucket %q: %v", p.bucketName, err)
		glog.Errorf(err.Error())
		return nil, err
	}

	// returned ob with connection info
	glog.Infof("Successfully created bucket %s", p.bucketName)
	return p.rtnObjectBkt(p.bucketName), nil
}

// Grant attaches to an existing aws s3 bucket and returns a connection info
// representing the bucket's endpoint and user access credentials.
func (p gcsProvisioner) Grant(options *apibkt.BucketOptions) (*v1alpha1.ObjectBucket, error) {
	return nil, nil
}

// Delete the bucket and all its objects.
// Note: only called when the bucket's reclaim policy is "delete".
func (p gcsProvisioner) Delete(ob *v1alpha1.ObjectBucket) error {

	p.bucketName = ob.Spec.Endpoint.BucketName

	// Delete the bucket
	ctx := context.Background()
	err := p.gcsClient.Bucket(p.bucketName).Delete(ctx)
	if err != nil {
		return err
	}
	glog.Infof("Bucket %s successfully deleted", p.bucketName)
	
	return nil
}

// Revoke removes a user, policy and access keys from an existing bucket.
func (p gcsProvisioner) Revoke(ob *v1alpha1.ObjectBucket) error {
	return nil
}

// create k8s config and client for the runtime-controller.
// Note: panics on errors.
func createConfigAndClientOrDie(masterurl, kubeconfig string) (*restclient.Config, *kubernetes.Clientset) {
	config, err := clientcmd.BuildConfigFromFlags(masterurl, kubeconfig)
	if err != nil {
		glog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}
	return config, clientset
}

func main() {
	defer glog.Flush()
	syscall.Umask(0)

	handleFlags()

	glog.Infof("GCS Bucket Provisioner - main")
	glog.V(2).Infof("flags: kubeconfig=%q; masterURL=%q", kubeconfig, masterURL)

	config, clientset := createConfigAndClientOrDie(masterURL, kubeconfig)

	stopCh := handleSignals()

	gcsProv := gcsProvisioner{}
	gcsProv.clientset = clientset

	// Create and run the s3 provisioner controller.
	// It implements the Provisioner interface expected by the bucket
	// provisioning lib.
	gcsProvisionerController, err := NewGcsProvisioner(config, gcsProv)
	if err != nil {
		glog.Errorf("killing GCS Bucket Provisioner, error initializing library controller: %v", err)
		os.Exit(1)
	}
	glog.V(2).Infof("main: running %s provisioner...", provisionerName)
	gcsProvisionerController.Run(stopCh)

	<-stopCh
	glog.Infof("main: %s provisioner exited.", provisionerName)
}

// Set -kubeconfig and (deprecated) -master flags.
// Note: when the bucket library used the controller-runtime, -kubeconfig and -master were
//   set its config package's init() function. Now this is done here.
func handleFlags() {

	flag.StringVar(&kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", os.Getenv("MASTER"), "(Deprecated: use `--kubeconfig`) The address of the Kubernetes API server. Overrides kubeconfig. Only required if out-of-cluster.")

	if !flag.Parsed() {
		flag.Parse()
	}
}

// Shutdown gracefully on system signals.
func handleSignals() <-chan struct{} {
	sigCh := make(chan os.Signal)
	stopCh := make(chan struct{})
	go func() {
		signal.Notify(sigCh)
		<-sigCh
		close(stopCh)
		os.Exit(1)
	}()
	return stopCh
}
