import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/auth/auth_crypto.dart';
import 'package:hnuhole_mobile/src/posts/post_api.dart';
import 'package:hnuhole_mobile/src/posts/post_http_transport.dart';

import 'loopback_tls_fixture.dart';

final _token = AuthCrypto.encode(List.filled(32, 1));
final _replacement = AuthCrypto.encode(List.filled(32, 2));
final _requestId = AuthCrypto.encode(List.filled(16, 3));
const _time = '2026-10-08T00:00:00Z';
const _expiry = '2026-10-08T01:00:00Z';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  // This isolate uses only its own real, certificate-verified TLS endpoint.
  HttpOverrides.global = null;
  late LoopbackTlsFixture tls;
  late HttpServer server;
  late PostHttpTransport transport;
  late Future<void> Function(HttpRequest) respond;
  final requests =
      <
        ({
          int port,
          String method,
          String? bearer,
          String body,
          bool persistent,
        })
      >[];

  setUpAll(() async => tls = await LoopbackTlsFixture.create());
  tearDownAll(() => tls.dispose());
  setUp(() async {
    requests.clear();
    server = await HttpServer.bindSecure(
      InternetAddress.loopbackIPv4,
      0,
      SecurityContext()
        ..useCertificateChainBytes(utf8.encode(tls.certificate))
        ..usePrivateKeyBytes(utf8.encode(tls.privateKey)),
    );
    server.listen((request) async {
      final port = request.connectionInfo!.remotePort;
      final body = await utf8.decoder.bind(request).join();
      requests.add((
        port: port,
        method: request.method,
        bearer: request.headers.value(HttpHeaders.authorizationHeader),
        body: body,
        persistent: request.persistentConnection,
      ));
      await respond(request);
    });
    transport = PostHttpTransport(
      communityBaseUri: Uri.parse('https://127.0.0.1:${server.port}'),
      client: HttpClient(
        context: SecurityContext(withTrustedRoots: false)
          ..setTrustedCertificatesBytes(utf8.encode(tls.certificate)),
      ),
    );
  });
  tearDown(() async {
    transport.close();
    await server.close(force: true);
  });

  Future<PostResponse<int>> get(String token) => transport.send(
    token,
    'GET',
    '/api/v1/me/feed-preferences',
    decode: (raw) => (raw as Map)['sequence'] as int,
  );

  Future<void> json(HttpRequest request, int status, Object body) async {
    final response = request.response;
    response.statusCode = status;
    response.headers
      ..contentType = ContentType.json
      ..set('Cache-Control', 'no-store')
      ..set('X-Request-ID', _requestId);
    if (status == 200) {
      response.headers
        ..set('Server-Time', _time)
        ..set('Session-Expires-At', _expiry);
    }
    response.write(jsonEncode(body));
    await response.close();
  }

  test('GETs reuse verified TLS but send each current bearer and read each response', () async {
    respond = (r) => json(r, 200, {'sequence': requests.length});
    expect((await get(_token)).value, 1);
    expect((await get(_replacement)).value, 2);
    expect(requests, hasLength(2));
    expect(requests.map((r) => r.port).toSet(), hasLength(1));
    expect(requests.map((r) => r.bearer), [
      'Bearer $_token',
      'Bearer $_replacement',
    ]);
    expect(requests.every((r) => r.persistent), isTrue);
  });

  test(
    '401 on a reused connection cannot return old protected content',
    () async {
      respond = (r) => requests.length == 2
          ? json(r, 401, {
              'error': {'code': 'SESSION_INVALID', 'message': 'Rejected'},
              'requestId': _requestId,
            })
          : json(r, 200, {'sequence': requests.length});
      expect((await get(_token)).value, 1);
      await expectLater(
        get(_token),
        throwsA(
          isA<PostFailure>()
              .having((e) => e.kind, 'kind', PostFailureKind.unauthorized)
              .having((e) => e.statusCode, 'status', 401),
        ),
      );
      expect((await get(_replacement)).value, 3);
      expect(requests, hasLength(3));
      expect(requests.map((r) => r.port).toSet(), hasLength(1));
      expect(requests.last.bearer, 'Bearer $_replacement');
    },
  );

  test(
    'truncated response fails and cannot contaminate the next TLS connection',
    () async {
      respond = (r) async {
        if (requests.length != 1) {
          await json(r, 200, {'sequence': requests.length});
          return;
        }
        final socket = await r.response.detachSocket(writeHeaders: false);
        socket.write(
          'HTTP/1.1 200 OK\r\n'
          'Content-Type: application/json\r\n'
          'Cache-Control: no-store\r\n'
          'X-Request-ID: $_requestId\r\n'
          'Server-Time: $_time\r\n'
          'Session-Expires-At: $_expiry\r\n'
          'Content-Length: 1000\r\n\r\n{"sequence":',
        );
        await socket.flush();
        await socket.close();
      };
      await expectLater(
        get(_token),
        throwsA(
          isA<PostFailure>().having(
            (e) => e.kind,
            'kind',
            PostFailureKind.transport,
          ),
        ),
      );
      expect((await get(_replacement)).value, 2);
      expect(requests, hasLength(2));
      expect(requests.first.port, isNot(requests.last.port));
    },
  );

  for (final method in ['POST', 'PUT']) {
    test('$method with keepAlive reuses the verified TLS connection', () async {
      respond = (r) => json(r, 200, {'sequence': requests.length});
      expect((await get(_token)).value, 1);
      await transport.send(
        _token,
        method,
        '/api/v1/chat/commands/fixed-command',
        body: {'operation': 'send'},
        keepAlive: true,
        decode: (raw) => raw,
      );
      expect(requests, hasLength(2));
      expect(requests.last.method, method);
      expect(requests.last.persistent, isTrue);
      expect(requests.last.port, requests.first.port);
      // The opt-in write leaves the connection usable for the next read.
      expect((await get(_replacement)).value, 3);
      expect(requests.last.port, requests.first.port);
    });
  }

  for (final method in ['POST', 'PUT', 'DELETE']) {
    test('$method received then disconnected is sent once after a reusable GET', () async {
      respond = (r) async {
        if (r.method == 'GET') {
          await json(r, 200, {'sequence': requests.length});
        } else {
          // The whole command body was received above; its outcome is unknown.
          final socket = await r.response.detachSocket(writeHeaders: false);
          await socket.close();
        }
      };
      expect((await get(_token)).value, 1);
      await expectLater(
        transport.send(
          _token,
          method,
          '/api/v1/post-commands/fixed-command',
          body: {'operation': 'original-intent'},
          decode: (raw) => raw,
        ),
        throwsA(
          isA<PostFailure>().having(
            (e) => e.kind,
            'kind',
            PostFailureKind.transport,
          ),
        ),
      );
      // A subsequent read proves no automatic command retry was queued.
      expect((await get(_replacement)).value, 3);
      final commands = requests.where((r) => r.method == method).toList();
      expect(commands, hasLength(1));
      expect(commands.single.persistent, isFalse);
      expect(jsonDecode(commands.single.body), {
        'operation': 'original-intent',
      });
      expect(requests.first.port, commands.single.port);
      expect(requests.last.port, isNot(commands.single.port));
    });
  }
}
