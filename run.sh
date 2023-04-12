#!/bin/sh

# First run the import program. It will read the db.dir from the config file in order to
# find an old v1.json. This will be converted to the new db format.

./bin/import
if [ $? -ne 0 ]; then
    exit 1
fi

# Run the FFmpeg migration program. In case a FFmpeg 5 binary is present, it will create a
# backup of the current DB and modify the FFmpeg parameter such that they are compatible
# with FFmpeg 5.

./bin/ffmigrate
if [ $? -ne 0 ]; then
    exit 1
fi

# Now run the core with the possibly converted configuration.

./bin/core
