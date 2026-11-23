# Chat review follow-up

Fix the staging chat findings from the review of PR #320.

1. Keep billable generation to one transport attempt per reserved round. Test a transport error and an explicit failure.
2. Block unverified appointment times and render requested prices from the catalog alongside calendar results. Test both behaviors through the reply handler.
3. Make the deployment dependency probe report the startup content load without starting a read. Test healthy, unavailable, and unstarted loads.
4. Regenerate the templates and locales, then run the quality gate and focused regression tests.
