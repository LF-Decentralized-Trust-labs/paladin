package cdomain

/*
#include <dlfcn.h>
#include <stdlib.h>

void callLoadDomain(void *handle, const unsigned char* data, unsigned int length) {
	void (*LoadDomain)(const unsigned char* data, unsigned int length);

    *(void **) (&LoadDomain) = dlsym(handle, "LoadDomain");
    if (LoadDomain) {
        LoadDomain(data, length);
    }
}

*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/hyperledger/firefly-common/pkg/log"
	"google.golang.org/protobuf/proto"

	"github.com/kalaeido-io/paladin/pkg/domain"
	protobuf "github.com/kalaeido-io/paladin/pkg/protobuf/pb"
	"github.com/kalaeido-io/paladin/pkg/transaction"
)

type cgoDomain struct {
}

func LoadDomainCLib(ctx context.Context, path string) (domain.Domain, error) {
	log.L(ctx).Infof("Loading %s", path)
	// Create an instance of the protobuf message
	msg := &protobuf.LoadDomainInput{
		DomainId:        "alice",
		FieldNames:      []string{"bob", "charlie"},
		IncludeMetadata: true,
	}

	// Serialize the message to a byte slice
	data, err := proto.Marshal(msg)
	if err != nil {
		log.L(ctx).Infof("failed to marshal message: %v", err)
		return nil, fmt.Errorf("failed to marshal message: %v", err)
	}

	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	// Open the shared library
	handle := C.dlopen(cPath, C.RTLD_LAZY)
	if handle == nil {
		return nil, fmt.Errorf("failed to open library: %s", path)
	}
	defer C.dlclose(handle)

	// Call the LoadDomain function
	C.callLoadDomain(handle, (*C.uchar)(unsafe.Pointer(&data[0])), C.uint(len(data)))

	return &cgoDomain{}, nil
}

func (domain *cgoDomain) Assemble(request transaction.Request) (transaction.Assembly, error) {
	return transaction.Assembly{}, nil
}

func (domain *cgoDomain) Define() (string, error) {
	return "default C domain definition", nil
}
