# `ocf` - Emulate Cloud Foundry's `cf` tool on OpenShift

This repository contains a command-line tool written in Go that
emulates the Cloud Foundry `cf` tool but targeting OpenShift. The
primary purpose of this tool is to ease migration of existing Cloud
Foundry applications and workflows to OpenShift.

The tool is still in its infancy and there are probably plenty of bugs
and untested corners. Speaking of tests, this could use some automated
ones as well.

This builds on top of the Docker image at
https://github.com/bbrowning/openshift-cloudfoundry that provides the
necessary buildpacks to run Cloud Foundry applications on
OpenShift. Any limitations that image has in emulating the Cloud
Foundry environment will apply when using this tool.

## Prerequisites

You'll need a built copy of this tool as well as the `oc` binaries
from OpenShift in your `$PATH`. Instructions on how to do that should
go here. Until then, if you're familiar with Go and Openshift it
shouldn't be hard to figure out how to do this.

## Example usage with Cloud Foundry's Spring Music sample

Clone https://github.com/cloudfoundry-samples/spring-music somewhere
locally and execute the following commands from inside its directory:

    ./gradlew assemble
    ocf push

The `ocf` tool will read the application's `manifest.yml` file and
deploy the application based on its contents. If everything goes as
planned, the final output of the command will show the URL assigned to
the application.

You'll notice these are identical commands to deploying it on Cloud
Foundry, except you replace `cf` with `ocf`.
