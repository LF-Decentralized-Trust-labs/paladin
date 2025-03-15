---
title: pgroup_*
---
## `pgroup_call`

### Parameters

0. `call`: [`PrivacyGroupEVMCall`](../types/privacygroupevmcall.md#privacygroupevmcall)

### Returns

0. `data`: [`RawJSON`](../types/simpletypes.md#rawjson)

## `pgroup_createGroup`

### Parameters

0. `spec`: [`PrivacyGroupInput`](../types/privacygroupinput.md#privacygroupinput)

### Returns

0. `group`: [`PrivacyGroup`](../types/privacygroup.md#privacygroup)

## `pgroup_createMessageListener`

### Parameters

0. `listener`: [`PrivacyGroupMessageListener`](../types/privacygroupmessagelistener.md#privacygroupmessagelistener)

### Returns

0. `success`: `bool`

## `pgroup_deleteMessageListener`

### Parameters

0. `listenerName`: `string`

### Returns

0. `success`: `bool`

## `pgroup_getGroupByAddress`

### Parameters

0. `address`: [`EthAddress`](../types/simpletypes.md#ethaddress)

### Returns

0. `pgroup`: [`PrivacyGroup`](../types/privacygroup.md#privacygroup)

## `pgroup_getGroupById`

### Parameters

0. `domainName`: `string`
1. `id`: [`HexBytes`](../types/simpletypes.md#hexbytes)

### Returns

0. `pgroup`: [`PrivacyGroup`](../types/privacygroup.md#privacygroup)

## `pgroup_getMessageById`

### Parameters

0. `id`: [`UUID`](../types/simpletypes.md#uuid)

### Returns

0. `msg`: [`PrivacyGroupMessage`](../types/privacygroupmessage.md#privacygroupmessage)

## `pgroup_getMessageListener`

### Parameters

0. `listenerName`: `string`

### Returns

0. `listener`: [`PrivacyGroupMessageListener`](../types/privacygroupmessagelistener.md#privacygroupmessagelistener)

## `pgroup_queryGroups`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

### Returns

0. `pgroups`: [`PrivacyGroup[]`](../types/privacygroup.md#privacygroup)

## `pgroup_queryGroupsWithMember`

### Parameters

0. `member`: `string`
1. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

### Returns

0. `pgroups`: [`PrivacyGroup[]`](../types/privacygroup.md#privacygroup)

## `pgroup_queryMessageListeners`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

### Returns

0. `listeners`: [`PrivacyGroupMessageListener[]`](../types/privacygroupmessagelistener.md#privacygroupmessagelistener)

## `pgroup_queryMessages`

### Parameters

0. `query`: [`QueryJSON`](../types/queryjson.md#queryjson)

### Returns

0. `msgs`: [`PrivacyGroupMessage[]`](../types/privacygroupmessage.md#privacygroupmessage)

## `pgroup_sendMessage`

### Parameters

0. `msg`: [`PrivacyGroupMessageInput`](../types/privacygroupmessageinput.md#privacygroupmessageinput)

### Returns

0. `msgId`: [`UUID`](../types/simpletypes.md#uuid)

## `pgroup_sendTransaction`

### Parameters

0. `tx`: [`PrivacyGroupEVMTXInput`](../types/privacygroupevmtxinput.md#privacygroupevmtxinput)

### Returns

0. `transactionId`: [`UUID`](../types/simpletypes.md#uuid)

## `pgroup_startMessageListener`

### Parameters

0. `listenerName`: `string`

### Returns

0. `success`: `bool`

## `pgroup_stopMessageListener`

### Parameters

0. `listenerName`: `string`

### Returns

0. `success`: `bool`

