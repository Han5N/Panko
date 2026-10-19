#include "bot/session.h"
#include "test.h"

#include <vector>

using panko::Event;
using panko::Session;

TEST_CASE(session_start_emits_event) {
    Session session("sim.local:19132", "PankoBot", 860);

    std::vector<Event> events;
    session.setEventSink([&](const Event& ev) { events.push_back(ev); });

    CHECK_EQ(session.status()["state"], std::string("offline"));
    session.start();

    CHECK_EQ(events.size(), size_t{1});
    CHECK_EQ(events[0].topic, std::string("session"));
    CHECK_EQ(events[0].seq, uint64_t{1});
    CHECK_EQ(session.status()["state"], std::string("online"));

    session.stop();
}

TEST_CASE(session_say_publishes_chat) {
    Session session("sim.local:19132", "PankoBot", 860);
    std::vector<Event> events;
    session.setEventSink([&](const Event& ev) { events.push_back(ev); });

    session.start();
    events.clear();

    auto result = session.say("hello");
    CHECK(result["ok"].get<bool>());
    CHECK_EQ(events.size(), size_t{1});
    CHECK_EQ(events[0].topic, std::string("chat"));
    CHECK_EQ(events[0].payload["message"], std::string("hello"));
}

TEST_CASE(session_say_offline_throws) {
    Session session("sim.local:19132", "PankoBot", 860);
    bool thrown = false;
    try {
        session.say("hello");
    } catch (const std::exception&) {
        thrown = true;
    }
    CHECK(thrown);
}

TEST_CASE(session_replay_since) {
    Session session("sim.local:19132", "PankoBot", 860);
    session.start();
    session.say("one");
    session.say("two");

    auto all = session.replay(0);
    CHECK_EQ(all.size(), size_t{3});

    auto tail = session.replay(all[1].seq);
    CHECK_EQ(tail.size(), size_t{1});
    CHECK_EQ(tail[0].payload["message"], std::string("two"));
}

TEST_CASE(session_block_query) {
    Session session("sim.local:19132", "PankoBot", 860);
    session.start();

    auto ground = session.block(0, 63, 0);
    CHECK(ground["loaded"].get<bool>());
    CHECK_EQ(ground["block"]["name"], std::string("minecraft:grass_block"));

    auto stone = session.block(0, 10, 0);
    CHECK(stone["loaded"].get<bool>());
    CHECK_EQ(stone["block"]["name"], std::string("minecraft:stone"));

    // 未加载的段不返回方块，避免把“没数据”误报成空气。
    auto unloaded = session.block(0, 200, 0);
    CHECK(!unloaded["loaded"].get<bool>());
    CHECK(!unloaded.contains("block"));
}

TEST_CASE(session_place_emits_block_events) {
    Session session("sim.local:19132", "PankoBot", 860);
    std::vector<Event> events;
    session.setEventSink([&](const Event& ev) { events.push_back(ev); });

    session.start();
    events.clear();

    nlohmann::json blocks = nlohmann::json::array({
        {{"position", {{"x", 1}, {"y", 64}, {"z", 2}}}, {"block", {{"name", "minecraft:stone"}}}},
    });

    auto result = session.place(blocks);
    CHECK_EQ(result["accepted"], 1);
    CHECK_EQ(events.size(), size_t{1});
    CHECK_EQ(events[0].topic, std::string("block"));
    CHECK_EQ(events[0].payload["after"]["name"], std::string("minecraft:stone"));
}
