package godomain

import (
	"context"
	"fmt"
	"plugin"

	"github.com/hyperledger/firefly-common/pkg/log"

	"github.com/kalaeido-io/paladin/pkg/domain"
)

// LoadDomainLib loads the domain library from the specified path.
// The path parameter specifies the path to the domain library file.
// It returns the loaded Domain and any error encountered during the loading process.
func LoadDomainLib(ctx context.Context, path string) (domain.Domain, error) {
	plug, err := plugin.Open(path)
	if err != nil {
		return nil, err
	}

	loadDomainSymbol, err := plug.Lookup("LoadDomain")
	if err != nil {
		return nil, err
	}

	loadDomainFunc, ok := loadDomainSymbol.(func() domain.Domain)
	if !ok {
		log.L(ctx).Infof("Domain symbol is not of type func")
		return nil, fmt.Errorf("domain symbol is not of type func")
	}

	loadedDomain := loadDomainFunc()

	return loadedDomain, nil
}
