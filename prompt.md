Context:
We have 2 repositories, clones of which are located in the same folder "Vibecoding Project". Vibecoding-guide-main is the main repository. Its key conditions are the complete absence of any AI artifacts: no Claude.md files, no .claude folders or other Claude-related files. This is the working repository of my project, which should have a "showcase" appearance — it should not be apparent that it was made with the help of an LLM. Using any git commands in this repository is strictly prohibited. There is 2 branches - main and stage.

For primary development and testing, there is Vibecoding-guide-dev. Anything can be stored in it — AI artifacts, local builds, intermediate files. Any git commands can be used here, as long as they are not related to the previous repository.

In this repo we need 4 branches
AI/Stage - development branch for stage-branch feautures
AI/Master - Development branch for developing master-branch features
Original/Stage - Full copy of the original main repo stage branch without any traces of ai.
Original/Master - Full copy of the original main repo main branch without any traces of ai.


The flow should look like that:

Git pull in the main repo -> Manually copy from main repo branch into Original/ branch in dev repo -> Pull changes from Orignal/ in AI/ -> Some work in AI/ branch by user -> merge into original/ without ai artifacts -> manually copy all that changes into main repo corresponding branch


Task:
It is necessary to set up this flow between these repos and branches using Claude Code skills.
Create 2 Claude Code Skills:

The first skill should manually pull all changes from the main working repository (main) into the vibecoding one (dev) first into Original/ branch and then into AI/ branch. (Analogous to Git pull)

The skill should force the user to run 'git pull' command before it start do something.

The second skill should push all changes we created during development in AI/ branches from the vibecoding repository into the main working repository into corresponding branch (main). (Analogous to Git push)

Make sure that adding, editing and deleting files works properly

After that, record the "Context" section described at the beginning of this prompt in claude.md. Compress it and leave only important parts  and the git pipeline.

