#!/bin/sh

# First run the import program. It will read the db.dir from the config file in order to
# find an old v1.json. This will be converted to the new db format.

./bin/import
if [ $? -ne 0 ]; then
    exit 1
fi

# Now run the core with the possibly converted configuration.

./bin/core
