// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

interface IPenteExternalCall {
    /**
     * This event may be emitted by private contracts running on top of Pente when they
     * wish to trigger a side-effect on a public smart contract.
     *
     * Any calls emitted here will be caught by the Pente domain logic and then passed
     * in to the transition() method in PentePrivacyGroup, such that the side-effects
     * will happen atomically with the Pente state transition.
     *
     * @param contractAddress the address of another contract on the shared base ledger
     * @param encodedCall an ABI-encoded function call to perform on the contract
     */
    event PenteExternalCall(address indexed contractAddress, bytes encodedCall);
}
