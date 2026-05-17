# Module camera-utils

A Viam module providing utility camera components that wrap and transform the output of other cameras. Currently this module ships a single model, `mask-camera`, which applies an image mask to a base camera so that only selected pixels (and corresponding point cloud points) are passed through.

## Models

This module provides the following model(s):

- [`viam-labs:camera-utils:mask-camera`](viam-labs_camera-utils_mask-camera.md) - Wraps an existing camera and applies a PNG mask to filter pixels in returned images and point clouds.
