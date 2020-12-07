local ans, has, cursor = {}, {}, "0"
repeat
    local t = redis.call("SCAN", cursor)
    local list = t[2]
    for i = 1, #list do
        local s = list[i]
        if has[s] == nil then
            has[s] = 1
            local value = redis.call("get", s)
            if value == "0" then
                ans[#ans + 1] = s
            end
        end
    end
    cursor = t[1]
until cursor == "0"
return ans --or return ans


-- eval "for i = 1, 200000 do redis.call('SET','authToken_' .. i,i) end" 0

-- local ans, has, cursor = {}, {}, "0"
-- repeat
--     local t = redis.call("SCAN", cursor)
--     local list = t[2]
--     for i = 1, #list do
--         local s = list[i]
--         if has[s] == nil then
--             has[s] = 1
--             ans[#ans + 1] = s
--         end
--     end
--     cursor = t[1]
-- until cursor == "0"
-- return #ans --or return ans