
#include <iostream>
#include "domainB.h"
#include "LoadDomain.pb.h"

using namespace std;
void LoadDomain(const unsigned char* data, unsigned int length){
    // Verify that the version of the library that we linked against is
    // compatible with the version of the headers we compiled against.
    GOOGLE_PROTOBUF_VERIFY_VERSION;
    std::cout<<"Hello from domain B++"<<std::endl;

    paladin::pkg::protobuf::LoadDomainInput loadDomainInput;

    std::string str(reinterpret_cast<const char*>(data), length);
    
    if (!loadDomainInput.ParseFromString(str)) {
      std::cerr << "Failed to parse LoadDomainInput" << std::endl;
    }
    for (int i = 0; i < loadDomainInput.field_names_size(); i++) {
      cout << "FieldName: " << loadDomainInput.field_names(i) << endl;
    }
}
