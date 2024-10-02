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

import hre from "hardhat";
import { IdentityRegistry } from "../../typechain-types/contracts/registry";
import { SignerWithAddress } from "@nomicfoundation/hardhat-ethers/signers";
import { expect } from "chai";
import { ContractTransactionResponse, EventLog } from "ethers";

/**
 * Test design
 * -----------
 *
 * The following identity hierarchy is registered:
 *
 *    root                  (owned by accounts[0])
 *    ├── identity-a        (owned by accounts[1])
 *    │   ├── identity-a-a  (owned by accounts[3])
 *    │   └── identity-a-b  (owned by accounts[4])
 *    └── identity-b        (owned by accounts[2])
 *
 * The following properties are set:
 *
 *   root      key=key-root-1, value=value-root-1/updated
 *             key=key-root-2, value=value-root-2
 *
 *   identity-a    key=key-identity-a-1, value=value-identity-a-1
 *                 key=key-identity-a-2, value=value-identity-a-2
 */

describe("Identity Registry", () => {
  let identityRegistry: IdentityRegistry;

  let root_account: SignerWithAddress;
  let account_a: SignerWithAddress;
  let account_b: SignerWithAddress;
  let account_a_a: SignerWithAddress;
  let account_a_b: SignerWithAddress;
  let other_account: SignerWithAddress;

  let identity_a_hash: string;
  let identity_b_hash: string;
  let identity_a_a_hash: string;
  let identity_a_b_hash: string;

  before(async () => {
    [root_account, account_a, account_b, account_a_a, account_a_b, other_account] = await hre.ethers.getSigners();
    const IdentityRegistry = await hre.ethers.getContractFactory("IdentityRegistry");
    identityRegistry = await IdentityRegistry.connect(root_account).deploy();
    await identityRegistry.waitForDeployment();
  });

  it("Root Identity", async () => {
    // Root identity is established as part of the smart contract deployment, name 'root' owned by root_account
    const rootIdentity = await identityRegistry.getRootIdentity();
    expect(rootIdentity.parent).to.equal(hre.ethers.ZeroHash);
    expect(rootIdentity.children.length).to.equal(0);
    expect(rootIdentity.name).to.equal("root");
    expect(rootIdentity.owner).to.equal(root_account);
  });

  it("Register identities", async () => {
    // Owner of root identity registers child identity "identity-a" and sets the ownership to account_a
    const transaction1 = await identityRegistry
      .connect(root_account)
      .registerIdentity(hre.ethers.ZeroHash, "identity-a", account_a);
    const event1 = await getEvent(transaction1);
    expect(event1?.fragment.name).to.equal("IdentityRegistered");
    expect(event1?.args[0]).to.equal(hre.ethers.ZeroHash);
    expect(event1?.args[2]).to.equal("identity-a");
    expect(event1?.args[3]).to.equal(account_a);
    identity_a_hash = event1?.args[1];

    // Owner of root identity registers child identity "identity-b" and sets the ownership to account_b
    const transaction2 = await identityRegistry
      .connect(root_account)
      .registerIdentity(hre.ethers.ZeroHash, "identity-b", account_b);
    const event2 = await getEvent(transaction2);
    expect(event2?.fragment.name).to.equal("IdentityRegistered");
    expect(event2?.args[0]).to.equal(hre.ethers.ZeroHash);
    expect(event2?.args[2]).to.equal("identity-b");
    expect(event2?.args[3]).to.equal(account_b);
    identity_b_hash = event2?.args[1];
  });

  it("Register nested identities", async () => {
    // Owner of identity "identity-a" registers child identity "identity-a-a"
    const transaction1 = await identityRegistry
      .connect(account_a)
      .registerIdentity(identity_a_hash, "identity-a-a", account_a_a);
    const event1 = await getEvent(transaction1);
    expect(event1?.fragment.name).to.equal("IdentityRegistered");
    expect(event1?.args[0]).to.equal(identity_a_hash);
    expect(event1?.args[2]).to.equal("identity-a-a");
    expect(event1?.args[3]).to.equal(account_a_a);
    identity_a_a_hash = event1?.args[1];

    // Owner of root identity registers child identity "identity-a-a" as a child of identity "identity-a"
    const transaction2 = await identityRegistry
      .connect(account_a)
      .registerIdentity(identity_a_hash, "identity-a-b", account_a_b.address);
    const event2 = await getEvent(transaction2);
    expect(event2?.fragment.name).to.equal("IdentityRegistered");
    expect(event2?.args[0]).to.equal(identity_a_hash);
    expect(event2?.args[2]).to.equal("identity-a-b");
    expect(event2?.args[3]).to.equal(account_a_b);
    identity_a_b_hash = event2?.args[1];
  });

  it("Traverse identity hierarchy", async () => {
    // Root identity must have identity-a and identity-b as children
    const rootIdentity = await identityRegistry.getRootIdentity();
    expect(rootIdentity.children.length).to.equal(2);
    expect(rootIdentity.children[0]).to.equal(identity_a_hash);
    expect(rootIdentity.children[1]).to.equal(identity_b_hash);

    // identity-a must have identity-a-a and identity-a-b as children, and the root identity as parent
    const identity_a = await identityRegistry.getIdentity(rootIdentity.children[0]);
    expect(identity_a.name).to.equal("identity-a");
    expect(identity_a.parent).to.equal(hre.ethers.ZeroHash);
    expect(identity_a.children.length).to.equal(2);
    expect(identity_a.children[0]).to.equal(identity_a_a_hash);
    expect(identity_a.children[1]).to.equal(identity_a_b_hash);

    // identity-b must have no children and the root identity as parent
    const identity_b = await identityRegistry.getIdentity(rootIdentity.children[1]);
    expect(identity_b.name).to.equal("identity-b");
    expect(identity_b.parent).to.equal(hre.ethers.ZeroHash);
    expect(identity_b.children.length).to.equal(0);

    // identity-a-a must have no children and identity-a as parent
    const identity_a_a = await identityRegistry.getIdentity(identity_a.children[0]);
    expect(identity_a_a.name).to.equal("identity-a-a");
    expect(identity_a_a.parent).to.equal(identity_a_hash);
    expect(identity_a_a.children.length).to.equal(0);

    // identity-a-b must have no children and identity-a as parent
    const identity_a_b = await identityRegistry.getIdentity(identity_a.children[1]);
    expect(identity_a_b.name).to.equal("identity-a-b");
    expect(identity_a_b.parent).to.equal(identity_a_hash);
    expect(identity_a_b.children.length).to.equal(0);
  });

  it("Permission checking for root identity", async () => {
    // Only the owner of the identity should be allowed to add child identities
    await expect(
      identityRegistry.connect(other_account).registerIdentity(hre.ethers.ZeroHash, "identity_x", other_account)
    ).to.be.revertedWith("Forbidden");
  });

  it("Permission checking for identity-a and identity-b", async () => {
    // Attempt to register an identity on identity-a owned by account_a using other_account
    await expect(
      identityRegistry.connect(other_account).registerIdentity(identity_a_hash, "identity_x", other_account)
    ).to.be.revertedWith("Forbidden");

    // Attempt to register an identity on identity-b owned by account_b using other_account
    await expect(
      identityRegistry.connect(other_account).registerIdentity(identity_b_hash, "identity_x", other_account)
    ).to.be.revertedWith("Forbidden");
  });

  it("Permission checking for identity-a-a and identity-a-b", async () => {
    // Attempt to register an identity on identity-a-a owned by account_a_a using other_account
    await expect(
      identityRegistry.connect(other_account).registerIdentity(identity_a_a_hash, "identity_x", other_account)
    ).to.be.revertedWith("Forbidden");

    // Attempt to register an identity on identity-a-a owned by account_a_b using other_account
    await expect(
      identityRegistry.connect(other_account).registerIdentity(identity_a_b_hash, "identity_x", other_account)
    ).to.be.revertedWith("Forbidden");
  });

  it("Root identity owner should only be allowed to add direct children", async () => {
    // Attempt to register grand-child identity
    await expect(
      identityRegistry.connect(other_account).registerIdentity(identity_a_hash, "identity_x", other_account)
    ).to.be.revertedWith("Forbidden");
  });

  it("Should not allow registration of identities with empty string", async () => {
    // Attempt to register an identity with name set to empty string
    await expect(
      identityRegistry.connect(root_account).registerIdentity(hre.ethers.ZeroHash, "", other_account)
    ).to.be.revertedWith("Name cannot be empty");
  });

  it("Set properties on root and identity-a", async () => {
    // Set property key="key-root-1" value="value-root-1" on root identity using owner root_account
    await expect(
      identityRegistry.connect(root_account).setIdentityProperty(hre.ethers.ZeroHash, "key-root-1", "value-root-1")
    )
      .to.emit(identityRegistry, "PropertySet")
      .withArgs(hre.ethers.ZeroHash, "key-root-1", "value-root-1");

    // Set property key="key-root-2" value="value-root-2" on root identity using owner root_account
    await expect(
      identityRegistry.connect(root_account).setIdentityProperty(hre.ethers.ZeroHash, "key-root-2", "value-root-2")
    )
      .to.emit(identityRegistry, "PropertySet")
      .withArgs(hre.ethers.ZeroHash, "key-root-2", "value-root-2");

    // Set property key="key-identity-a-1" value="value-identity-a-1" on identity-a using owner account_a
    await expect(
      identityRegistry.connect(account_a).setIdentityProperty(identity_a_hash, "key-identity-a-1", "value-identity-a-1")
    )
      .to.emit(identityRegistry, "PropertySet")
      .withArgs(identity_a_hash, "key-identity-a-1", "value-identity-a-1");

    // Set property key="key-identity-a-2" value="value-identity-a-2" on identity-a using owner account_a
    await expect(
      identityRegistry.connect(account_a).setIdentityProperty(identity_a_hash, "key-identity-a-2", "value-identity-a-2")
    )
      .to.emit(identityRegistry, "PropertySet")
      .withArgs(identity_a_hash, "key-identity-a-2", "value-identity-a-2");
  });

  it("Lookup property values by key", async () => {
    // Property key="key-root-1" must have value="value-root-1" on root identity
    const transaction1 = await identityRegistry.getIdentityPropertyValueByName(hre.ethers.ZeroHash, "key-root-1");
    expect(transaction1).to.equal("value-root-1");

    // Property key="key-root-2" must have value="value-root-2" on root identity
    const transaction2 = await identityRegistry.getIdentityPropertyValueByName(hre.ethers.ZeroHash, "key-root-2");
    expect(transaction2).to.equal("value-root-2");

    // Property key="key-identity-a-1" must have value="value-identity-a-1" on identity-a
    const transaction3 = await identityRegistry.getIdentityPropertyValueByName(identity_a_hash, "key-identity-a-1");
    expect(transaction3).to.equal("value-identity-a-1");

    // Property key="key-identity-a-2" must have value="value-identity-a-2" on identity-a
    const transaction4 = await identityRegistry.getIdentityPropertyValueByName(identity_a_hash, "key-identity-a-2");
    expect(transaction4).to.equal("value-identity-a-2");
  });

  it("List properties", async () => {
    // Get property key hashes for root identity
    const transaction1 = await identityRegistry.listIdentityPropertyHashes(hre.ethers.ZeroHash);
    expect(transaction1.length).to.equal(2);

    // Get property key="key-root-1" using retreived key hash
    const transaction2 = await identityRegistry.getIdentityPropertyByHash(hre.ethers.ZeroHash, transaction1[0]);
    expect(transaction2[0]).to.equal("key-root-1");
    expect(transaction2[1]).to.equal("value-root-1");

    // Get property key="key-root-2" using retreived key hash
    const transaction3 = await identityRegistry.getIdentityPropertyByHash(hre.ethers.ZeroHash, transaction1[1]);
    expect(transaction3[0]).to.equal("key-root-2");
    expect(transaction3[1]).to.equal("value-root-2");

    // Get property key hashes for identity-a
    const transaction4 = await identityRegistry.listIdentityPropertyHashes(identity_a_hash);
    expect(transaction4.length).to.equal(2);

    // Get property key="key-identity-a-1" using retreived key hash
    const transaction5 = await identityRegistry.getIdentityPropertyByHash(identity_a_hash, transaction4[0]);
    expect(transaction5[0]).to.equal("key-identity-a-1");
    expect(transaction5[1]).to.equal("value-identity-a-1");

    // Get property key="key-identity-a-2" using retreived key hash
    const transaction6 = await identityRegistry.getIdentityPropertyByHash(identity_a_hash, transaction4[1]);
    expect(transaction6[0]).to.equal("key-identity-a-2");
    expect(transaction6[1]).to.equal("value-identity-a-2");
  });

  it("Check only identity owner can set properties", async () => {
    // Attempt to set property on root identity owned by root_account using other_account
    await expect(
      identityRegistry.connect(other_account).setIdentityProperty(hre.ethers.ZeroHash, "key-x", "value-x")
    ).to.be.revertedWith("Forbidden");

    // Attempt to set property on identity-b owned by account_b using other_account
    await expect(
      identityRegistry.connect(other_account).setIdentityProperty(identity_b_hash, "key-x", "value-x")
    ).to.be.revertedWith("Forbidden");
  });

  it("Should allow properties to be updated", async () => {
    // Update property key="key-root-1" setting value="updated" on root identity using root_account
    await expect(
      identityRegistry.connect(root_account).setIdentityProperty(hre.ethers.ZeroHash, "key-root-1", "updated")
    )
      .to.emit(identityRegistry, "PropertySet")
      .withArgs(hre.ethers.ZeroHash, "key-root-1", "updated");

    // Check value is updated
    const transaction2 = await identityRegistry.getIdentityPropertyValueByName(hre.ethers.ZeroHash, "key-root-1");
    expect(transaction2).to.equal("updated");
  });

  it("Properties should be available to all identities", async () => {
    // Access property in root identity from other_account
    const transaction1 = await identityRegistry
      .connect(other_account)
      .getIdentityPropertyValueByName(hre.ethers.ZeroHash, "key-root-1");
    expect(transaction1).to.equal("updated");

    // Access property in identity-a identity from other_account
    const transaction2 = await identityRegistry
      .connect(other_account)
      .getIdentityPropertyValueByName(identity_a_hash, "key-identity-a-1");
    expect(transaction2).to.equal("value-identity-a-1");
  });

  it("Should not allow empty string name properties", async () => {
    // Attempt to set a property on root identity with name="" using root_account
    await expect(
      identityRegistry.connect(root_account).setIdentityProperty(hre.ethers.ZeroHash, "", "value")
    ).to.be.revertedWith("Name cannot be empty");
  });

  it("Ensure there siblings have unique names", async () => {
    // Attempt to register a new child identity with repeated name
    await expect(
      identityRegistry.connect(root_account).registerIdentity(hre.ethers.ZeroHash, "identity-a", other_account)
    ).to.be.revertedWith("Name already taken");
  });

  it("Handle property not found errors", async () => {
    // Attempt to get property with the same hash as the identity
    await expect(identityRegistry.getIdentityPropertyByHash(identity_a_hash, identity_a_hash)).to.be.revertedWith(
      "Property not found"
    );
  });

  it("Remove identity", async () => {
    // Attempt to remove identity-a-a by its owner (account_a_a)
    const transaction = await identityRegistry.connect(account_a_a).removeIdentity(identity_a_a_hash);
    const event = await getEvent(transaction);
    expect(event?.fragment.name).to.equal("IdentityRemoved");
    expect(event?.args[0]).to.equal(identity_a_hash); // parent hash
    expect(event?.args[1]).to.equal(identity_a_a_hash); // removed identity hash
    expect(event?.args[2]).to.equal("identity-a-a"); // name
    expect(event?.args[3]).to.equal(account_a_a.address); // owner

    // Verify that identity-a-a no longer exists
    await expect(identityRegistry.getIdentity(identity_a_a_hash)).to.be.revertedWith("Identity not found");

    // Verify that identity-a-a is no longer a child of identity-a
    const identity_a = await identityRegistry.getIdentity(identity_a_hash);
    expect(identity_a.children).to.not.include(identity_a_a_hash);
    expect(identity_a.children.length).to.equal(1);

    // Add back identity-a-a
    const transaction1 = await identityRegistry
      .connect(account_a)
      .registerIdentity(identity_a_hash, "identity-a-a", account_a_a);
    const event1 = await getEvent(transaction1);
    expect(event1?.fragment.name).to.equal("IdentityRegistered");
    expect(event1?.args[0]).to.equal(identity_a_hash);
    expect(event1?.args[2]).to.equal("identity-a-a");
    expect(event1?.args[3]).to.equal(account_a_a);
    identity_a_a_hash = event1?.args[1];
  });

  it("Cannot remove non-existent identity", async () => {
    // Attempt to remove an identity that doesn't exist
    await expect(identityRegistry.connect(other_account).removeIdentity(hre.ethers.randomBytes(32))).to.be.revertedWith(
      "Identity not found"
    );
  });

  it("Only owner can remove identity", async () => {
    // Attempt to remove identity-a-b by non-owner (other_account)
    await expect(identityRegistry.connect(other_account).removeIdentity(identity_a_b_hash)).to.be.revertedWith(
      "Forbidden"
    );
  });

  it("Deleting identity with properties does not affect another identity and its properties", async () => {
    // Set the same property name on both identity-a and identity-b
    await identityRegistry.connect(account_a_a).setIdentityProperty(identity_a_a_hash, "test-key", "value-a");
    await identityRegistry.connect(account_a_b).setIdentityProperty(identity_a_b_hash, "test-key", "value-b");

    // Remove identity-a-a
    await identityRegistry.connect(account_a_a).removeIdentity(identity_a_a_hash);

    // Verify that identity-a-a no longer exists
    await expect(identityRegistry.getIdentity(identity_a_a_hash)).to.be.revertedWith("Identity not found");

    // Verify that the property is no longer accessible for identity-a-a
    await expect(identityRegistry.getIdentityPropertyValueByName(identity_a_a_hash, "test-key")).to.be.revertedWith(
      "Property not found"
    );

    // Verify that the property still exists for identity-a-b
    expect(await identityRegistry.getIdentityPropertyValueByName(identity_a_b_hash, "test-key")).to.equal("value-b");
  });

  it("Cannot remove root identity", async () => {
    // Attempt to remove the root identity
    await expect(identityRegistry.connect(root_account).removeIdentity(hre.ethers.ZeroHash)).to.be.revertedWith(
      "Cannot remove root identity"
    );
  });
});

const getEvents = async (response: ContractTransactionResponse) => {
  const receipt = await response.wait();
  return receipt?.logs?.filter((log) => log instanceof EventLog) as EventLog[];
};

const getEvent = async (response: ContractTransactionResponse) => {
  const events = await getEvents(response);
  return events.pop();
};
