import { createEmptyProject } from '@icm/model'
import { serializeProject } from '@icm/project-protocol'
const project = createEmptyProject('project-1', 'New Circuit', 'document-1')
process.stdout.write(serializeProject(project))
