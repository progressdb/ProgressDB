---
section: service
title: "Configuration"
order: 2
visibility: public
---

# Configuration

To use the `config.yaml`, it is recommended that you use secret or protected files—available in most cloud services.
- Both environment variables and config.yaml are checked and merged.
- If you set the same value in both config.yaml and the environment (env), the env value takes precedence.
- Keep your KMS embedded hex master key secure and backed up.
  - If you lose it, your data can't be decrypted.
  - For example, store it in infisical.com or a password manager.

<SpecFile file="config.yaml" title="Complete Configuration" />

<SpecFile file="env" title="Environment Variables" />

<!-- 
<SpecFile
  file="openapi.yaml"
  title="API Specification"
  collapsible={true}
/> -->