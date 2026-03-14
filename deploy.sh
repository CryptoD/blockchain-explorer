#!/bin/bash

# Deployment script for Blockchain Explorer

# Set variables
IMAGE_TAG=${1:-latest}
NAMESPACE=${2:-default}

echo "Deploying Blockchain Explorer with image tag: $IMAGE_TAG to namespace: $NAMESPACE"

# Build and push Docker image (assuming Docker is configured)
echo "Building Docker image..."
docker build -t blockchain-explorer:$IMAGE_TAG .

# For cloud deployment, you would push to a registry like ECR, GCR, etc.
# Example for AWS ECR:
# aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin <account>.dkr.ecr.us-east-1.amazonaws.com
# docker tag blockchain-explorer:$IMAGE_TAG <account>.dkr.ecr.us-east-1.amazonaws.com/blockchain-explorer:$IMAGE_TAG
# docker push <account>.dkr.ecr.us-east-1.amazonaws.com/blockchain-explorer:$IMAGE_TAG

# Update the image in Kubernetes manifests
sed -i "s|image: blockchain-explorer:latest|image: blockchain-explorer:$IMAGE_TAG|g" k8s/deployment.yaml

# Apply Kubernetes manifests
echo "Applying Kubernetes manifests..."
kubectl apply -f k8s/ -n $NAMESPACE

# Wait for rollout
kubectl rollout status deployment/blockchain-explorer -n $NAMESPACE

echo "Deployment complete. Check the service for external access."