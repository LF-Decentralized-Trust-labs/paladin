package main

import (
	"context"
	"os"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/log"

	"github.com/kalaeido-io/paladin/internal/cdomain"
	"github.com/kalaeido-io/paladin/internal/godomain"
	"github.com/kalaeido-io/paladin/pkg/domain"
)

func main() {
	ctx := context.Background()
	//DOMAIN_PATH is a colon separated list of file paths to .so files that export the domain interface

	domainPluginPath := os.Getenv("DOMAIN_PLUGIN_PATH")
	domainLibs := strings.Split(domainPluginPath, ":")
	domainCLibPath := os.Getenv("DOMAIN_PATH")
	domainCLibs := strings.Split(domainCLibPath, ":")
	domains := make([]domain.Domain, len(domainCLibs)+len(domainLibs))
	for _, domainLib := range domainLibs {
		log.L(ctx).Infof("Loading %s", domainLib)

		if domainLib == "" {
			continue
		}
		loadedDomain, err := godomain.LoadDomainLib(ctx, domainLib)
		if err != nil {
			log.L(ctx).Error("Failed to load domain library", err)
			return
		}
		log.L(ctx).Infof("Loaded %s", domainLib)
		domains = append(domains, loadedDomain)
	}

	for _, domainCLib := range domainCLibs {
		log.L(ctx).Infof("Loading %s", domainCLib)
		if domainCLib == "" {
			continue
		}
		loadedDomain, err := cdomain.LoadDomainCLib(ctx, domainCLib)
		if err != nil {
			log.L(ctx).Error("Failed to load domain C library", err)
			return
		}
		log.L(ctx).Infof("Loaded %s", domainCLib)
		domains = append(domains, loadedDomain)
	}

	for i, domain := range domains {
		if domain == nil {
			log.L(ctx).Infof("Domain %d nil ", i)
			continue
		}
		domainDefinition, err := domain.Define()
		if err != nil {
			log.L(ctx).Error("Failed to define domain", err)
			return
		}
		log.L(ctx).Infof("Domain defined: %s", domainDefinition)
	}

}
