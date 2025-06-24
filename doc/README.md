# GitSyncer Documentation

Welcome to the GitSyncer documentation. This directory contains comprehensive documentation about the GitSyncer project structure, architecture, and API reference.

## Table of Contents

1. [Architecture Overview](architecture.md) - High-level system design and architecture
2. [API Reference](api-reference.md) - Complete reference of all packages, types, and functions
3. [Configuration Guide](configuration.md) - How to configure GitSyncer
4. [Usage Examples](examples.md) - Common usage patterns and examples
5. [Development Guide](development.md) - Guide for contributors

## Quick Links

- [Project README](../README.md) - Main project documentation
- [Source Code](https://codeberg.org/snonux/gitsyncer) - Repository on Codeberg

## Overview

GitSyncer is a tool for synchronizing Git repositories across multiple platforms (GitHub, Codeberg, self-hosted Git servers). It supports:

- Bidirectional synchronization between multiple Git hosts
- Automatic branch management and filtering
- Repository creation on supported platforms
- Conflict detection and reporting
- Abandoned branch analysis

## Getting Started

1. Install GitSyncer
2. Create a configuration file
3. Run `gitsyncer --sync-all` to sync all configured repositories

See the [Configuration Guide](configuration.md) for detailed setup instructions.