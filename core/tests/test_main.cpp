#include "test.h"

#include <cstdio>
#include <exception>

namespace panko::test {

std::vector<Case>& registry() {
    static std::vector<Case> cases;
    return cases;
}

Registrar::Registrar(const std::string& name, std::function<void()> fn) {
    registry().push_back({name, std::move(fn)});
}

int runAll() {
    int failed = 0;
    for (const auto& item : registry()) {
        try {
            item.fn();
            std::printf("[ ok ] %s\n", item.name.c_str());
        } catch (const std::exception& e) {
            std::printf("[fail] %s\n       %s\n", item.name.c_str(), e.what());
            ++failed;
        }
    }
    std::printf("\n%d 个用例，%d 个失败\n", static_cast<int>(registry().size()), failed);
    return failed == 0 ? 0 : 1;
}

}  // namespace panko::test

int main() { return panko::test::runAll(); }
