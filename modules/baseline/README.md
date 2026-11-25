# baseline

Writes a security baseline on one VergeOS system: system settings, a TLS certificate, users, groups, permissions, and API keys that expire.

`profile` is `baseline` or `strict`. The `controls` output is the control mapping. `applied` is true when this configuration writes that control. `documented_value` is the product-guide default when that guide publishes one. Those defaults are not an approved compliance profile. Replace them with the values from the customer's security configuration guide.

| Control | strict | What it writes | Product-guide default |
| --- | --- | --- | --- |
| login_lockout_attempts | required when settings are applied | vergeio_setting for "Failed login attempts before an account is locked (0 disables lockout)" | 0, which disables lockout |
| login_lockout_minutes | required when settings are applied | vergeio_setting for "Minutes an account is locked for" | 15 |
| session_expiration | required when settings are applied | vergeio_setting for "Session expiration (seconds)" | 259200 |
| session_inactivity | required when settings are applied | vergeio_setting for "Session inactivity timeout (seconds)" | 86400 |
| logon_banner_header | required when settings are applied | vergeio_setting for "Logon banner header" | empty |
| logon_banner_text | required when settings are applied | vergeio_setting for "Logon banner text" | empty |
| password_complexity | required when settings are applied | vergeio_setting for "Password Complexity Requirement" | length:8 |
| tls_certificate | required | vergeio_certificate | |
| admin_accounts | required | vergeio_user, vergeio_group, vergeio_member, vergeio_permission | |
| api_key_expiry | when a key is declared | vergeio_api_key.expires | |

`settings` is keyed by those control ids. `key` is the string VergeOS stores. The provider names `max_connections` as one key that exists. It does not publish the stored names for lockout, session, banner, or password complexity. Read each key on the system and pass it here. A key VergeOS does not have fails apply. `vergeio_setting` updates an existing row. It does not create settings and it does not delete them.

Applying settings changes the parent system. Destroy writes each key's `default_value` back. That is the product default, which can differ from the value stored before this module ran. `value` is kept in state. A secret setting is a separate `vergeio_setting` with `value_wo`.

`apply_settings` false writes no settings. The lab test uses that. A plan of this module with `apply_settings` true is the change to the parent system.

`vergeio_certificate` stores a certificate row. The provider has no attribute that selects the certificate the UI serves. `certificate_private_keys` is the manual PEM private key, sent as `private_key_wo` and not stored.

Users, groups, memberships, and permissions are objects this module creates. A membership in a group that already exists, including Administrators, is a `vergeio_member` next to the module. Passwords go in `passwords`, keyed by user name. They are sent as `password_wo` and are not stored. A password needs an uppercase letter, a lowercase letter, a digit, and 8 characters. `password_complexity` on the system is a separate setting. The documented default is `length:8`. The stored spelling of a stricter rule is the value the system already accepts.

Each API key sets `expires`. VergeOS has no system setting for a default key lifetime. The bearer token comes back only on create, and `vergeio_api_key` does not store it. `api_key_max_lifetime_seconds` fails the plan when `expires` is further out than that many seconds. A key with no expiry is a `vergeio_api_key` next to the module.

A later plan is the drift record. `terraform plan -detailed-exitcode` exits 0 when the configuration still matches and exits 2 when something changed. The diff is the record. Run that plan on a schedule in the system that holds this state. This repository does not schedule it. The provider has no continuous check.

```hcl
module "baseline" {
  source = "../../modules/baseline"

  profile = "strict"

  settings = {
    login_lockout_attempts = {
      key   = var.login_lockout_attempts_key
      value = "0"
    }
    session_expiration = {
      key   = var.session_expiration_key
      value = "259200"
    }
  }

  certificates = [{
    name        = "ui"
    type        = "self_signed"
    domain_name = "ui.example.com"
    key_type    = "ecdsa"
  }]

  groups = [{
    name = "operators"
    permissions = [{
      table  = "vms"
      list   = true
      read   = true
      create = false
      modify = false
      delete = false
    }]
  }]

  users = [{
    name   = "ops"
    groups = ["operators"]
    api_keys = [{
      name    = "ci"
      expires = 1893456000
    }]
  }]

  passwords = {
    ops = var.ops_password
  }
}
```

A runnable root module is in `examples/modules/baseline`.

`terraform test` in this directory plans with a mocked provider. The lab apply test is a separate root in `tests/lab` and is not part of that run. It sets `apply_settings` to false and does not write system settings:

```bash
terraform -chdir=tests/lab init -backend=false
terraform -chdir=tests/lab test
```
