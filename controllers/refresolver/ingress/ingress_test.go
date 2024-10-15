package ingress

/*
Copyright 2022 The k8gb Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Generated by GoLic, for more details see: https://github.com/AbsaOSS/golic
*/

import (
	"testing"

	k8gbv1beta1 "github.com/k8gb-io/k8gb/api/v1beta1"
	"github.com/k8gb-io/k8gb/controllers/utils"
	"github.com/stretchr/testify/assert"
)

func TestGetServers(t *testing.T) {
	var tests = []struct {
		name            string
		ingressFile     string
		expectedServers []*k8gbv1beta1.Server
	}{
		{
			name:        "single server",
			ingressFile: "../testdata/ingress_referenced.yaml",
			expectedServers: []*k8gbv1beta1.Server{
				{
					Host: "ingress-referenced.cloud.example.com",
					Services: []*k8gbv1beta1.NamespacedName{
						{
							Name:      "ingress-referenced",
							Namespace: "test-gslb",
						},
					},
				},
			},
		},
		{
			name:        "multiple servers",
			ingressFile: "./testdata/ingress_multiple_servers.yaml",
			expectedServers: []*k8gbv1beta1.Server{
				{
					Host: "h1.cloud.example.com",
					Services: []*k8gbv1beta1.NamespacedName{
						{
							Name:      "s1",
							Namespace: "test-gslb",
						},
					},
				},
				{
					Host: "h2.cloud.example.com",
					Services: []*k8gbv1beta1.NamespacedName{
						{
							Name:      "ss1",
							Namespace: "test-gslb",
						},
						{
							Name:      "ss2",
							Namespace: "test-gslb",
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// arrange
			ingress := utils.FileToIngress(test.ingressFile)
			resolver := ReferenceResolver{
				ingress: ingress,
			}

			// act
			servers, err := resolver.GetServers()
			assert.NoError(t, err)

			// assert
			assert.Equal(t, test.expectedServers, servers)
		})
	}
}

func TestGetGslbExposedIPs(t *testing.T) {
	var tests = []struct {
		name        string
		ingressYaml string
		expectedIPs []string
	}{
		{
			name:        "no exposed IPs",
			ingressYaml: "./testdata/ingress_no_ips.yaml",
			expectedIPs: []string{},
		},
		{
			name:        "single exposed IP",
			ingressYaml: "../testdata/ingress_referenced.yaml",
			expectedIPs: []string{"10.0.0.1"},
		},
		{
			name:        "multiple exposed IPs",
			ingressYaml: "./testdata/ingress_multiple_ips.yaml",
			expectedIPs: []string{"10.0.0.1", "10.0.0.2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// arrange
			ingress := utils.FileToIngress(test.ingressYaml)
			resolver := ReferenceResolver{
				ingress: ingress,
			}

			// act
			IPs, err := resolver.GetGslbExposedIPs([]utils.DNSServer{})
			assert.NoError(t, err)

			// assert
			assert.Equal(t, test.expectedIPs, IPs)
		})
	}
}