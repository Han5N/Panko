cmake_minimum_required(VERSION 3.20)
project(panko_core VERSION 0.2.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(MSVC)
  add_compile_options(/W4 /utf-8)
else()
  add_compile_options(-Wall -Wextra -Wpedantic)
endif()

find_package(Threads REQUIRED)

add_library(panko_core STATIC
  src/common/buffer.cpp
  src/protocol/packets.cpp
  src/bot/session.cpp
  src/control/server.cpp
  src/world/block_registry.cpp
  src/world/subchunk.cpp
  src/world/chunk_store.cpp
)

target_include_directories(panko_core PUBLIC
  ${CMAKE_CURRENT_SOURCE_DIR}/src
  ${CMAKE_CURRENT_SOURCE_DIR}/third_party
)

target_link_libraries(panko_core PUBLIC Threads::Threads)
set(PANKO_VERSION "${PROJECT_VERSION}" CACHE STRING "Panko 版本号")
target_compile_definitions(panko_core PUBLIC PANKO_VERSION="${PANKO_VERSION}")

if(WIN32)
  # 控制接口使用 Winsock，静态库的使用方也需要链接。
  target_link_libraries(panko_core PUBLIC ws2_32)
endif()

add_executable(botd src/main.cpp)
target_link_libraries(botd PRIVATE panko_core)

include(CTest)
if(BUILD_TESTING)
  add_executable(core_tests
    tests/test_main.cpp
    tests/buffer_test.cpp
    tests/packet_test.cpp
    tests/session_test.cpp
    tests/world_test.cpp
  )
  target_link_libraries(core_tests PRIVATE panko_core)
  add_test(NAME core_tests COMMAND core_tests)
endif()
