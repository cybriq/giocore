# giocore

This repository is a stripped down version of 
[Gio](https://gioui.org) with all optional parts removed and 
only the core (window, gpu, ops and events) in order to form the back end 
for a new widget/app/layout system called [pokaz](https://github.com/cybriq/pokaz).

The library will be updated to incorporate upstream patches from time to 
time to maintain parity with the latest core updates, but it will not be 
strictly up to date, there may be a lag of up to 3 months behind the main 
from time to time.

This repository is synced up to 
[gioui.org](https://git.sr.ht/~eliasnaur/gio) 
with everything pertaining to the `layout/` and `widget/` folders removed, 
and the odd thing that somehow depends on these.

Note that the `main` branch of this repository matches the upstream, this 
repository's default branch is `master` which is the same except omitting 
the `widget` and `widget/material` packages. The content of these from 
upstream is being adapted to permit a separate map and render for laying out 
viewports where the layout is arbitrarily displaced and clipped into another 
widget.
