#!/bin/bash

set -e

image_name="unfs:var_lib_apt_lists_cache"
container_name="unfs_var_lib_apt_lists_cache"
mount_name=$container_name

# cleanup
echo "Cleaning up ..."
docker volume rm $mount_name >/dev/null 2>&1      || true
docker rm --force $container_name >/dev/null 2>&1 || true
docker rmi --force $image_name >/dev/null 2>&1    || true

# create nfs-server image
echo "Creating nfs-server image ..."
docker build --tag $image_name -<<EOF
FROM macadmins/unfs3
RUN mkdir -p /cache
RUN echo -n "/cache (rw,no_root_squash)" > /etc/exports
EOF

# run nfs-server
echo "Running nfs-server ..."
docker run --name=$container_name $image_name &
until docker logs $container_name 2>/dev/null | grep -q "ip 0.0.0.0 mask 0.0.0.0"; do
   echo ".";
   sleep 1;
done

# create volume
echo "Creating volume ..."
container_ip=$(docker inspect --format="{{ .NetworkSettings.IPAddress }}" $container_name)
[[ -z "$container_ip" ]] && (echo "УВАГА"; exit 1)
docker volume create --driver local --opt type=nfs --opt o=addr="$container_ip",rw,nolock --opt device=:/cache $mount_name

# fill cache
echo "Cache filling ..."
docker run --rm -v $mount_name:/var/lib/apt/lists ubuntu:latest bash -c "apt-get update && apt-get install curl -y"

# run tests
echo "Running tests ..."
docker run --rm ubuntu:latest bash -c "apt-get install curl -y" || echo "TEST 1 [OK]"
docker run --rm -v $mount_name:/var/lib/apt/lists ubuntu:latest bash -c "apt-get install curl -y" && echo "TEST 2 [OK]"

# cleanup
echo "Cleaning up ..."
docker volume rm $mount_name >/dev/null 2>&1      || true
docker rm --force $container_name >/dev/null 2>&1 || true
docker rmi --force $image_name >/dev/null 2>&1    || true
