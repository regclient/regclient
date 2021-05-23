# Lua modules

This directory contains Lua modules packaged into the docker image of regbot.
They may be used with the following syntax in a script:

```lua
modname = require 'modname' -- this would import modname.lua
modname.someFunc(args)
```
