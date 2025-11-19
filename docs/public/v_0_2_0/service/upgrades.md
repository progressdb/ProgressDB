---
section: service
title: "Upgrades"
order: 4
visibility: public
---

# Upgrades

ProgressDB automatically migrates and performs any necessary work for new versions to function properly. This allows you to deploy a new version and have it automatically ready for new data.

## Recommended Safety Procedures

For maximum safety, follow these steps:

1. **Backup your database** - Save the entire database folder (contains everything the database needs)
2. **Upgrade the version** - Deploy the new version
3. **Check the logs** - Look for `migration_complete` or `new version saved` log messages

## Error Handling

If errors occur during migration:

1. **Restore** the backed up database folder & instance
2. **Report the issue** to the repository

## Migration Notes

- Migrations are extensively tested to ensure reliability
- Changes primarily focus on architecture rather than core data modifications
- Starting from version 0.2.0, folder structures have been standardized

## Downtime Considerations

Migrations require pausing load on the active server during the upgrade process. This limitation is noted as a TODO for future version releases to improve real-world workflow compatibility.