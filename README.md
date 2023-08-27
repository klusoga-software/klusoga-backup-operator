# Klusoga Backup Operator

## Description:
This Operator manages backup targets and destinations for the klusoga backup cli tool

## MssqlTargets:
```yaml
apiVersion: backup.klusoga.de/v1alpha1
kind: MssqlTarget
metadata:
  name: mssqltarget-sample
spec:
  credentialsRef: mssql-creds
  destinationRef: s3
  image: "klusoga/backup:v0.1.4"
  port: "1433"
  host: "172.20.134.0"
  path: /mssql-backup/backup
  schedule: "*/5 * * * *"
  databases: master
  persistentVolumeClaimName: backup-claim
```

A MssqlTarget resource manages backups of Microsoft SQL backups.

| Parameter                 | Description                                                                      |
|---------------------------|----------------------------------------------------------------------------------|
| credentialsRef            | A secret that contains username and password of the sql server for backups       |
| destinationRef            | The name of the destination resource you want to send your backups to            |
| image                     | The docker image of the klusoga backup cli tool you want to use                  |
| port                      | The port of the mssql server                                                     |
| host                      | The service IP adress of the mssql server                                        |
| path                      | The path to store mssql backups. This needs to be mounted to a ReadWriteMany PVC |
| schedule                  | The cronjob schedule you want to backup the databases                            |
| databases                 | A comma separated list of databases you want to backup                           |
| persistentVolumeClaimName | The name of the pvc the backup volume is mounted to                              |
