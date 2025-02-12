package assistant

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
	"context"
	coreerrors "errors"
	"fmt"

	"github.com/k8gb-io/k8gb/controllers/utils"

	"github.com/k8gb-io/k8gb/controllers/logging"

	"github.com/miekg/dns"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	externaldns "sigs.k8s.io/external-dns/endpoint"
)

const coreDNSServiceLabel = "app.kubernetes.io/name=coredns"

// Gslb is common wrapper operating on GSLB instance.
// It uses apimachinery client to call kubernetes API
type Gslb struct {
	client         client.Client
	k8gbNamespace  string
	edgeDNSServers utils.DNSList
}

var log = logging.Logger()

func NewGslbAssistant(client client.Client, k8gbNamespace string, edgeDNSServers []utils.DNSServer) *Gslb {
	return &Gslb{
		client:         client,
		k8gbNamespace:  k8gbNamespace,
		edgeDNSServers: edgeDNSServers,
	}
}

// GetCoreDNSService returns the CoreDNS Service
func (r *Gslb) GetCoreDNSService() (*corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	sel, err := labels.Parse(coreDNSServiceLabel)
	if err != nil {
		log.Err(err).Msg("Badly formed label selector")
		return nil, err
	}
	listOption := &client.ListOptions{
		LabelSelector: sel,
		Namespace:     r.k8gbNamespace,
	}

	err = r.client.List(context.TODO(), serviceList, listOption)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Warn().Err(err).Msg("Can't find CoreDNS service")
		}
	}
	if len(serviceList.Items) != 1 {
		log.Warn().Msg("More than 1 CoreDNS service was found")
		for _, service := range serviceList.Items {
			log.Info().
				Str("serviceName", service.Name).
				Msg("Found CoreDNS service")
		}
		err := coreerrors.New("more than 1 CoreDNS service was found. Check if CoreDNS exposed correctly")
		return nil, err
	}
	coreDNSService := &serviceList.Items[0]
	return coreDNSService, nil
}

// CoreDNSExposedIPs retrieves list of IP's exposed by CoreDNS
func (r *Gslb) CoreDNSExposedIPs() ([]string, error) {
	coreDNSService, err := r.GetCoreDNSService()
	if err != nil {
		return nil, err
	}
	if coreDNSService.Spec.Type == "ClusterIP" {
		if len(coreDNSService.Spec.ClusterIPs) == 0 {
			errMessage := "no ClusterIPs found"
			log.Warn().
				Str("serviceName", coreDNSService.Name).
				Msg(errMessage)
			err := coreerrors.New(errMessage)
			return nil, err
		}
		return coreDNSService.Spec.ClusterIPs, nil
	}
	// LoadBalancer / ExternalName / NodePort service
	var lb corev1.LoadBalancerIngress
	if len(coreDNSService.Status.LoadBalancer.Ingress) == 0 {
		errMessage := "no LoadBalancer ExternalIPs are found"
		log.Warn().
			Str("serviceName", coreDNSService.Name).
			Msg(errMessage)
		err := coreerrors.New(errMessage)
		return nil, err
	}
	lb = coreDNSService.Status.LoadBalancer.Ingress[0]
	return extractIPFromLB(lb, r.edgeDNSServers)
}

func extractIPFromLB(lb corev1.LoadBalancerIngress, ns utils.DNSList) (ips []string, err error) {
	if lb.Hostname != "" {
		IPs, err := utils.Dig(lb.Hostname, 8, ns...)
		if err != nil {
			log.Warn().Err(err).
				Str("loadBalancerHostname", lb.Hostname).
				Msg("Can't dig CoreDNS service LoadBalancer FQDN")
			return nil, err
		}
		return IPs, nil
	}
	if lb.IP != "" {
		return []string{lb.IP}, nil
	}
	return nil, nil
}

// SaveDNSEndpoint update DNS endpoint or create new one if doesnt exist
func (r *Gslb) SaveDNSEndpoint(namespace string, i *externaldns.DNSEndpoint) error {
	found := &externaldns.DNSEndpoint{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Name:      i.Name,
		Namespace: namespace,
	}, found)
	if err != nil && errors.IsNotFound(err) {

		// Create the DNSEndpoint
		log.Info().
			Interface("DNSEndpoint", i).
			Msgf("Creating a new DNSEndpoint")
		err = r.client.Create(context.TODO(), i)

		if err != nil {
			// Creation failed
			log.Err(err).
				Str("namespace", i.Namespace).
				Str("name", i.Name).
				Msg("Failed to create new DNSEndpoint")
			return err
		}
		// Creation was successful
		return nil
	} else if err != nil {
		// Error that isn't due to the service not existing
		log.Err(err).Msg("Failed to get DNSEndpoint")
		return err
	}

	// Update existing object with new spec, labels and annotations
	found.Spec = i.Spec
	found.ObjectMeta.Annotations = i.ObjectMeta.Annotations
	found.ObjectMeta.Labels = i.ObjectMeta.Labels
	err = r.client.Update(context.TODO(), found)

	if err != nil {
		// Update failed
		log.Err(err).
			Str("namespace", found.Namespace).
			Str("name", found.Name).
			Msg("Failed to update DNSEndpoint")
		return err
	}
	return nil
}

// RemoveEndpoint removes endpoint
func (r *Gslb) RemoveEndpoint(endpointName string) error {
	log.Info().
		Str("namespace", r.k8gbNamespace).
		Str("name", endpointName).
		Msg("Removing endpoint")
	dnsEndpoint := &externaldns.DNSEndpoint{}
	err := r.client.Get(context.Background(), client.ObjectKey{Namespace: r.k8gbNamespace, Name: endpointName}, dnsEndpoint)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Warn().
				Str("namespace", r.k8gbNamespace).
				Str("name", endpointName).
				Err(err).
				Msg("Endpoint not found")
			return nil
		}
		return err
	}
	err = r.client.Delete(context.TODO(), dnsEndpoint)
	return err
}

func getARecords(msg *dns.Msg) []string {
	var ARecords []string
	for _, nsA := range msg.Answer {
		ip := nsA.(*dns.A).A.String()
		ARecords = append(ARecords, ip)
	}
	return ARecords
}

func dnsQuery(host string, nameservers utils.DNSList) (*dns.Msg, error) {
	dnsMsg := new(dns.Msg)
	fqdn := fmt.Sprintf("%s.", host) // Convert to true FQDN with dot at the end
	dnsMsg.SetQuestion(fqdn, dns.TypeA)
	dnsMsgA, err := utils.Exchange(dnsMsg, nameservers)
	if err != nil {
		log.Warn().
			Str("fqdn", fqdn).
			Interface("nameservers", nameservers).
			Err(err).
			Msg("can't resolve FQDN using nameservers")
	}
	return dnsMsgA, err
}

func (r *Gslb) GetExternalTargets(host string, extClusterNsNames map[string]string) (targets Targets) {
	targets = NewTargets()
	for tag, cluster := range extClusterNsNames {
		// Use edgeDNSServer for resolution of NS names and fallback to local nameservers
		log.Info().
			Str("cluster", cluster).
			Msg("Adding external Gslb targets from cluster")
		glueA, err := dnsQuery(cluster, r.edgeDNSServers)
		if err != nil {
			return targets
		}
		log.Info().
			Str("nameserver", cluster).
			Interface("edgeDNSServers", r.edgeDNSServers).
			Interface("glueARecord", glueA.Answer).
			Msg("Resolved glue A record for NS")
		glueARecords := getARecords(glueA)
		var hostToUse string
		if len(glueARecords) > 0 {
			hostToUse = glueARecords[0]
		} else {
			hostToUse = cluster
		}
		nameServersToUse := getNSCombinations(r.edgeDNSServers, hostToUse)
		lHost := fmt.Sprintf("localtargets-%s", host)
		a, err := dnsQuery(lHost, nameServersToUse)
		if err != nil {
			return targets
		}
		clusterTargets := getARecords(a)
		if len(clusterTargets) > 0 {
			targets[tag] = &Target{clusterTargets}
			log.Info().
				Strs("clusterTargets", clusterTargets).
				Str("cluster", cluster).
				Msg("Extend Gslb targets by targets from cluster")
		}
	}
	return targets
}

func getNSCombinations(original []utils.DNSServer, hostToUse string) []utils.DNSServer {
	portToUse := original[0].Port
	nameServerToUse := []utils.DNSServer{
		{
			Host: hostToUse,
			Port: portToUse,
		},
	}
	defaultPortAdded := false
	for _, s := range original {
		if s.Port != 53 {
			nameServerToUse = append(nameServerToUse, utils.DNSServer{
				Host: hostToUse,
				Port: s.Port,
			})
		} else {
			defaultPortAdded = true
		}
	}
	if !defaultPortAdded {
		nameServerToUse = append(nameServerToUse, utils.DNSServer{
			Host: hostToUse,
			Port: 53,
		})
	}
	return nameServerToUse
}
