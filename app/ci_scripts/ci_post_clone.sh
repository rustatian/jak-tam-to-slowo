#!/bin/sh
set -e

# Xcode Cloud cannot show the trust question for a package build plugin. This setting lets the OpenAPI generator plugin run.
# "Validatation" is the spelling that Xcode reads.
# https://swiftpackageindex.com/apple/swift-openapi-generator/documentation/swift-openapi-generator/frequently-asked-questions#How-do-I-enable-the-build-plugin-in-Xcode-and-Xcode-Cloud
defaults write com.apple.dt.Xcode IDESkipPackagePluginFingerprintValidatation -bool YES
