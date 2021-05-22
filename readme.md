# Gio Core

This repository is a stripped down version of [Gio](https://gioui.org) with
all optional parts removed and only the core (window, gpu, ops and events)
in order to form the back end for a new, yet to be named widget and layout
system loosely derived from the layout/widget/material packages formerly
found in here.

The library will be updated to incorporate upstream patches from time to 
time to maintain parity with the latest core updates, but it will not be
strictly up to date, there may be a lag of up to 3 months behind the main
from time to time.

This repository will still contain the `main` branch from gioui.org but
the default is set to `master` which will be what the github servers provide
when the branch is not specified.