# User Interface

## Introduction

The graphical user interface makes it easy to visualize activity in Paladin nodes. This includes transactions, events, submissions and node registry.


## Accessing the UI

Each Paladin node runs an instance of the Paladin UI at the path `/ui`. If you used the provided kind config
(or a similar port mapping) with devnet, you should be able to access the UI for each node:

* http://localhost:31548/ui
* http://localhost:31648/ui
* http://localhost:31748/ui

## Interacting with the UI

### Indexer

The indexer panel displays recent transactions and events. 

<img src="../../images/ui-indexer.png" alt="Indexer" style=" box-shadow: 0px 0px 15px 0px rgba(0,0,0,0.57">

Clicking on `View Details` brings up detailed information on a transaction or event:

<img src="../../images/ui-transaction.png" alt="Transaction" style=" box-shadow: 0px 0px 15px 0px rgba(0,0,0,0.57">

### Submissions

The submissions panel displays recent transactions that have been submitted with an option to display only those in pending state.

<img src="../../images/ui-submissions.png" alt="Submissions" style=" box-shadow: 0px 0px 15px 0px rgba(0,0,0,0.57">

### Registry

The registry panel displays the list of Paladin nodes.

<img src="../../images/ui-registry.png" alt="Registry" style=" box-shadow: 0px 0px 15px 0px rgba(0,0,0,0.57">
