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

	"fmt"
	_ "net/url"

	"github.com/golang/glog"

	storageV1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Return the region name from the passed in storage class.
func getRegion(sc *storageV1.StorageClass) string {

	const scRegionKey = "region"
	return sc.Parameters[scRegionKey]
}

// Return the region name from the passed in storage class.
func getProject(sc *storageV1.StorageClass) string {

	const scProjectKey = "project"
	return sc.Parameters[scProjectKey]
}

// Return the storage class for a given name.
func (p *gcsProvisioner) getClassByNameForBucket(className string) (*storageV1.StorageClass, error) {

	glog.V(2).Infof("getting storage class %q...", className)
	class, err := p.clientset.StorageV1().StorageClasses().Get(className, metav1.GetOptions{})
	// TODO: retry w/ exponential backoff
	if err != nil {
		return nil, fmt.Errorf("unable to Get storageclass %q: %v", className, err)
	}
	return class, nil
}