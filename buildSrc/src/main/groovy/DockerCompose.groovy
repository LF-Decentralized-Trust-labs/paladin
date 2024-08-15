import org.gradle.api.GradleException
import org.gradle.api.tasks.Exec
import org.gradle.process.ExecResult

class DockerCompose extends Exec {

    private String _composeFile = 'docker-compose.yml'

    DockerCompose() {
        ignoreExitValue true
    }

    void composeFile(String f) {
        this._composeFile = f
    }

    private String findExecutable() {
        String dockerComposeV2Check = 'docker compose version'.execute().text
        if (dockerComposeV2Check.contains('Docker Compose')) {
            executable = 'docker'
            args = ['compose', *args]
            return 'docker compose'
        }
        executable = 'docker-compose'
        return 'docker-compose'
    }

    @Override
    protected void exec() {
        args = ['-f', _composeFile, *args]
        String composeCommand = findExecutable()

        super.exec()

        ExecResult execResult = executionResult.get()
        if (execResult.exitValue != 0) {
            println "${composeCommand} -f ${_composeFile} logs"
                .execute()
                .waitForProcessOutput(System.err, System.err)
            throw new GradleException('Docker compose failed')
        }
    }

}
