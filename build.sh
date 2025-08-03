#!/bin/bash
docker build -t registry.satnusa.com/developer/itop-ldap-department-synchronizer:latest .
docker push registry.satnusa.com/developer/itop-ldap-department-synchronizer:latest