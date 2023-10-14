# iac-pulumi

This repo contains the code for bringing up the pulumi infrastructure specified in aws.

# Build Instructions

1. Download all dependencies using `go mod download` .

# Deploy Instructions

1. Go the iac-pulumi directory and execute `pulumi up` .
2. To bring up infrastructure in a specific stack , use `pulumi up -s` .<stackname>`
3. To remove the stack , execute `pulumi destroy` .
4. All the configs needed are to be given in corresponding stack yaml.
   
   