---
title: PrivacyGroupInput
---
{% include-markdown "./_includes/privacygroupinput_description.md" %}

### Example

```json
{
    "domain": "",
    "members": null,
    "name": ""
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `domain` | The domain of the privacy group | `string` |
| `members` | The member list must be a set of fully qualified identity locators 'some.identity@node.name' | `string[]` |
| `name` | Optional name for the privacy group, which is indexed for efficient query | `string` |
| `properties` | Application specific properties for the privacy group | `` |
| `configuration` | Domain specific configuration options that define the behavior of the privacy group | `` |
| `transactionOptions` | Options that will be propagated to the final private transaction that is submitted after the domain has validated the input properties and generated the base private transaction | [`PrivacyGroupTXOptions`](#privacygrouptxoptions) |

## PrivacyGroupTXOptions


