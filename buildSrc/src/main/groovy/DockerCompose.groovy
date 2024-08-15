import org.gradle.api.GradleException
import org.gradle.api.tasks.Exec
import org.gradle.process.ExecResult

class DockerCompose extends Exec {

    private List<String> _composeFiles = []

    DockerCompose() {
        ignoreExitValue true
    }

    void composeFile(String f) {
        _composeFiles += '-f'
        _composeFiles += f
    }

    void dumpLogs(String service = "") {
        if (service == "") {
            println "Dumping Docker logs"
        } else {
            println "Dumping Docker logs for ${service}"
        }
        "${dockerCommand().join(' ')} logs ${service}"
            .execute().waitForProcessOutput(System.err, System.err)
    }

    private List<String> dockerCommand() {
        if (_composeFiles.size() == 0) {
            _composeFiles = ['-f', 'docker-compose.yml']
        }
        String dockerComposeV2Check = 'docker compose version'.execute().text
        if (dockerComposeV2Check.contains('Docker Compose')) {
            return ['docker', 'compose', *_composeFiles]
        }
        return ['docker-compose', *_composeFiles]
    }

    @Override
    protected void exec() {
        List<String> cmd = dockerCommand()
        executable = cmd.remove(0)
        args = [*cmd, *args]

        super.exec()

        ExecResult execResult = executionResult.get()
        if (execResult.exitValue != 0) {
            dumpLogs()
        }
        execResult.assertNormalExitValue()
    }

}
