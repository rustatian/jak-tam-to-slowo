#!/bin/sh
set -e

# Xcode Cloud cannot show the trust question for a package build plugin. This setting lets the OpenAPI generator plugin run.
# "Validatation" is the spelling that Xcode reads.
# https://swiftpackageindex.com/apple/swift-openapi-generator/documentation/swift-openapi-generator/frequently-asked-questions#How-do-I-enable-the-build-plugin-in-Xcode-and-Xcode-Cloud
defaults write com.apple.dt.Xcode IDESkipPackagePluginFingerprintValidatation -bool YES

# A build that a tag starts takes its version from the tag: v0.39.1 gives 0.39.1.
# https://developer.apple.com/documentation/xcode/environment-variable-reference#Variable-for-tag-changes
if [ -n "$CI_TAG" ]; then
	sed -i '' "s/MARKETING_VERSION = .*;/MARKETING_VERSION = ${CI_TAG#v};/" "$CI_PRIMARY_REPOSITORY_PATH/app/Slowo.xcodeproj/project.pbxproj"
fi
