---
section: service
title: "Configuration"
order: 2
visibility: public
---

# Configuration

To use the config.yaml - it is recommended you use secret or protected files - provided in most cloud services.
- Config.yaml takes prescedence over env variables
- Both are checked and merged with config.yaml set values taking prescendence.
- Keep your kms embdedded hex master key secure and backed up 
  - if you loose it, your data can't be decrypted
  - e.g put it in infisical.com or password protectors

<SpecFile file="config.yaml" title="Complete Configuration" />

<SpecFile file="env" title="Environment Variables" />
<!-- 
<SpecFile
  file="openapi.yaml"
  title="API Specification"
  collapsible={true}
/> -->