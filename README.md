# gcp-bucket-provisioner

### Build and Push the image

1. Build the provisioner binary.
```
 # go build -o ./bin/gcs-provisioner  ./cmd/...
```

2. Login to docker and quay.io.
```
 # docker login
 # docker login quay.io
```

3. Build the image and push it to quay.io.
```
 # docker build . -t quay.io/<your_quay_account>/<your prov>:<tag>
 # docker push quay.io/<your_quay_account>/<your prov>:<tag>
```

i.e.

```
 # docker build . -t quay.io/screeley44/gcs-bucket-provisioner:v1.0.0
 # docker push quay.io/screeley44/gcs-bucket-provisioner:v1.0.0
```
