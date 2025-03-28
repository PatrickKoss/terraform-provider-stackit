---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "stackit_public_ip Resource - stackit"
subcategory: ""
description: |-
  Public IP resource schema. Must have a region specified in the provider configuration.
---

# stackit_public_ip (Resource)

Public IP resource schema. Must have a `region` specified in the provider configuration.

## Example Usage

```terraform
resource "stackit_public_ip" "example" {
  project_id           = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  network_interface_id = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  labels = {
    "key" = "value"
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `project_id` (String) STACKIT project ID to which the public IP is associated.

### Optional

- `labels` (Map of String) Labels are key-value string pairs which can be attached to a resource container
- `network_interface_id` (String) Associates the public IP with a network interface or a virtual IP (ID). If you are using this resource with a Kubernetes Load Balancer or any other resource which associates a network interface implicitly, use the lifecycle `ignore_changes` property in this field to prevent unintentional removal of the network interface due to drift in the Terraform state

### Read-Only

- `id` (String) Terraform's internal resource ID. It is structured as "`project_id`,`public_ip_id`".
- `ip` (String) The IP address.
- `public_ip_id` (String) The public IP ID.
