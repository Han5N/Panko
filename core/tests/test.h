#pragma once

#include <functional>
#include <stdexcept>
#include <string>
#include <vector>

namespace panko::test {

struct Case {
    std::string name;
    std::function<void()> fn;
};

std::vector<Case>& registry();

struct Registrar {
    Registrar(const std::string& name, std::function<void()> fn);
};

int runAll();

}  // namespace panko::test

#define TEST_CASE(name)                                              \
    static void name();                                              \
    static ::panko::test::Registrar reg_##name(#name, name);         \
    static void name()

#define CHECK(cond)                                                              \
    do {                                                                         \
        if (!(cond)) {                                                           \
            throw std::runtime_error(std::string("CHECK 失败: ") + #cond);       \
        }                                                                        \
    } while (0)

#define CHECK_EQ(a, b)                                                                       \
    do {                                                                                     \
        auto _a = (a);                                                                       \
        auto _b = (b);                                                                       \
        if (!(_a == _b)) {                                                                   \
            throw std::runtime_error(std::string("CHECK_EQ 失败: ") + #a + " != " + #b);     \
        }                                                                                    \
    } while (0)
