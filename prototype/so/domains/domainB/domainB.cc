/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <iostream>
#include "domainB.h"
#include "Domain.pb.h"

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
