#! /bin/sh

( cat ~/.config/argos/.notifications; echo "·" ) | perl -ne 'if (/\S/) { print; exit }'
echo ---
