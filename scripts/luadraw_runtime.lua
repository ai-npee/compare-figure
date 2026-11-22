-- Standalone LuaDraw launcher used by luadraw_runtime.py.
local files = assert(arg[1], "LuaDraw files directory required")
local source = assert(arg[2], "figure.lua required")
local output = assert(arg[3], "output directory required")

package.path = files .. "/?.lua;" .. files .. "/extensions/?.lua;" .. package.path

table.append = table.append or function(t, ...)
    for _, v in ipairs({...}) do
        if type(v) == "table" then
            for _, item in ipairs(v) do t[#t + 1] = item end
        else
            t[#t + 1] = v
        end
    end
    return t
end

table.copy = table.copy or function(t)
    local copy = {}
    for key, value in pairs(t) do copy[key] = value end
    return copy
end

local original_require = require
require = function(name)
    if type(name) == "string" then name = name:gsub("%.lua$", "") end
    return original_require(name)
end

tex = tex or { sprint = function(...) end, print = print }
luadraw = { cachedir = output .. "/" }
luadraw.graph = require "luadraw_graph2d"
function luadraw.graph:Show() self:Savetofile(luadraw.graph_name) end
function luadraw.graph:Save() self:Savetofile(luadraw.graph_name) end
luadraw.graph3d = require "luadraw_graph3d"
require "luadraw_frustum_and_co"
require "luadraw_central_perspective"
ld = luadraw
Z = luadraw.cpx.Z
M = luadraw.pt3d.M
pt3d = luadraw.pt3d
local output_name = arg[4] or "figure"
luadraw.graph_name = output .. "/" .. output_name .. ".tkz"

local chunk, err = loadfile(source)
assert(chunk, err)
local ok, result = pcall(chunk)
if not ok then error(result) end
