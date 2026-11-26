# keyforge-data

The shared KeyForge card data for the keyforge.cards sites, vex, and other
projects.

This module holds:

- The card model: `Card`, `Printing`, and `Localization`.
- The card dataset, embedded in the module.
- The Master Vault crawler and the raw store.
- The distillation step, which makes the dataset from the raw store.
- The overrides for known Master Vault mistakes and labels.
- The query engine (`query`), which the search sites share.
- The icons, which the sites serve from the public bucket.

Read [CONTEXT.md](CONTEXT.md) for the vocabulary. Read [docs/adr](docs/adr) for
the design decisions. Read [docs/todo.md](docs/todo.md) for the work plan.

The card text, images, and icons are the property of Ghost Galaxy. This project
is a fan project and is not affiliated with Ghost Galaxy.
