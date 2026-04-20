/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars*/
import * as $protobuf from "protobufjs/minimal";

// Common aliases
const $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;

// Exported root namespace
const $root = $protobuf.roots["default"] || ($protobuf.roots["default"] = {});

export const pb = $root.pb = (() => {

    /**
     * Namespace pb.
     * @exports pb
     * @namespace
     */
    const pb = {};

    pb.RegisterRequest = (function() {

        /**
         * Properties of a RegisterRequest.
         * @memberof pb
         * @interface IRegisterRequest
         * @property {number|null} [pid] RegisterRequest pid
         */

        /**
         * Constructs a new RegisterRequest.
         * @memberof pb
         * @classdesc Represents a RegisterRequest.
         * @implements IRegisterRequest
         * @constructor
         * @param {pb.IRegisterRequest=} [properties] Properties to set
         */
        function RegisterRequest(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * RegisterRequest pid.
         * @member {number} pid
         * @memberof pb.RegisterRequest
         * @instance
         */
        RegisterRequest.prototype.pid = 0;

        /**
         * Creates a new RegisterRequest instance using the specified properties.
         * @function create
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.IRegisterRequest=} [properties] Properties to set
         * @returns {pb.RegisterRequest} RegisterRequest instance
         */
        RegisterRequest.create = function create(properties) {
            return new RegisterRequest(properties);
        };

        /**
         * Encodes the specified RegisterRequest message. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @function encode
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.IRegisterRequest} message RegisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterRequest.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.pid);
            return writer;
        };

        /**
         * Encodes the specified RegisterRequest message, length delimited. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.IRegisterRequest} message RegisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterRequest.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer.
         * @function decode
         * @memberof pb.RegisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.RegisterRequest} RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterRequest.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.RegisterRequest();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.RegisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.RegisterRequest} RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterRequest.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a RegisterRequest message.
         * @function verify
         * @memberof pb.RegisterRequest
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        RegisterRequest.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            return null;
        };

        /**
         * Creates a RegisterRequest message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.RegisterRequest
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.RegisterRequest} RegisterRequest
         */
        RegisterRequest.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.RegisterRequest)
                return object;
            let message = new $root.pb.RegisterRequest();
            if (object.pid != null)
                message.pid = object.pid >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a RegisterRequest message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.RegisterRequest
         * @static
         * @param {pb.RegisterRequest} message RegisterRequest
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        RegisterRequest.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.pid = 0;
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            return object;
        };

        /**
         * Converts this RegisterRequest to JSON.
         * @function toJSON
         * @memberof pb.RegisterRequest
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        RegisterRequest.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for RegisterRequest
         * @function getTypeUrl
         * @memberof pb.RegisterRequest
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        RegisterRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.RegisterRequest";
        };

        return RegisterRequest;
    })();

    pb.RegisterResponse = (function() {

        /**
         * Properties of a RegisterResponse.
         * @memberof pb
         * @interface IRegisterResponse
         * @property {boolean|null} [success] RegisterResponse success
         * @property {string|null} [message] RegisterResponse message
         */

        /**
         * Constructs a new RegisterResponse.
         * @memberof pb
         * @classdesc Represents a RegisterResponse.
         * @implements IRegisterResponse
         * @constructor
         * @param {pb.IRegisterResponse=} [properties] Properties to set
         */
        function RegisterResponse(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * RegisterResponse success.
         * @member {boolean} success
         * @memberof pb.RegisterResponse
         * @instance
         */
        RegisterResponse.prototype.success = false;

        /**
         * RegisterResponse message.
         * @member {string} message
         * @memberof pb.RegisterResponse
         * @instance
         */
        RegisterResponse.prototype.message = "";

        /**
         * Creates a new RegisterResponse instance using the specified properties.
         * @function create
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.IRegisterResponse=} [properties] Properties to set
         * @returns {pb.RegisterResponse} RegisterResponse instance
         */
        RegisterResponse.create = function create(properties) {
            return new RegisterResponse(properties);
        };

        /**
         * Encodes the specified RegisterResponse message. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @function encode
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.IRegisterResponse} message RegisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterResponse.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.success);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            return writer;
        };

        /**
         * Encodes the specified RegisterResponse message, length delimited. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.IRegisterResponse} message RegisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        RegisterResponse.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer.
         * @function decode
         * @memberof pb.RegisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.RegisterResponse} RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterResponse.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.RegisterResponse();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.success = reader.bool();
                        break;
                    }
                case 2: {
                        message.message = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.RegisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.RegisterResponse} RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        RegisterResponse.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a RegisterResponse message.
         * @function verify
         * @memberof pb.RegisterResponse
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        RegisterResponse.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.success != null && message.hasOwnProperty("success"))
                if (typeof message.success !== "boolean")
                    return "success: boolean expected";
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            return null;
        };

        /**
         * Creates a RegisterResponse message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.RegisterResponse
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.RegisterResponse} RegisterResponse
         */
        RegisterResponse.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.RegisterResponse)
                return object;
            let message = new $root.pb.RegisterResponse();
            if (object.success != null)
                message.success = Boolean(object.success);
            if (object.message != null)
                message.message = String(object.message);
            return message;
        };

        /**
         * Creates a plain object from a RegisterResponse message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.RegisterResponse
         * @static
         * @param {pb.RegisterResponse} message RegisterResponse
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        RegisterResponse.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.success = false;
                object.message = "";
            }
            if (message.success != null && message.hasOwnProperty("success"))
                object.success = message.success;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            return object;
        };

        /**
         * Converts this RegisterResponse to JSON.
         * @function toJSON
         * @memberof pb.RegisterResponse
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        RegisterResponse.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for RegisterResponse
         * @function getTypeUrl
         * @memberof pb.RegisterResponse
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        RegisterResponse.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.RegisterResponse";
        };

        return RegisterResponse;
    })();

    pb.UnregisterRequest = (function() {

        /**
         * Properties of an UnregisterRequest.
         * @memberof pb
         * @interface IUnregisterRequest
         * @property {number|null} [pid] UnregisterRequest pid
         */

        /**
         * Constructs a new UnregisterRequest.
         * @memberof pb
         * @classdesc Represents an UnregisterRequest.
         * @implements IUnregisterRequest
         * @constructor
         * @param {pb.IUnregisterRequest=} [properties] Properties to set
         */
        function UnregisterRequest(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * UnregisterRequest pid.
         * @member {number} pid
         * @memberof pb.UnregisterRequest
         * @instance
         */
        UnregisterRequest.prototype.pid = 0;

        /**
         * Creates a new UnregisterRequest instance using the specified properties.
         * @function create
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.IUnregisterRequest=} [properties] Properties to set
         * @returns {pb.UnregisterRequest} UnregisterRequest instance
         */
        UnregisterRequest.create = function create(properties) {
            return new UnregisterRequest(properties);
        };

        /**
         * Encodes the specified UnregisterRequest message. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @function encode
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.IUnregisterRequest} message UnregisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterRequest.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.pid);
            return writer;
        };

        /**
         * Encodes the specified UnregisterRequest message, length delimited. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.IUnregisterRequest} message UnregisterRequest message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterRequest.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer.
         * @function decode
         * @memberof pb.UnregisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.UnregisterRequest} UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterRequest.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.UnregisterRequest();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.UnregisterRequest
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.UnregisterRequest} UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterRequest.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an UnregisterRequest message.
         * @function verify
         * @memberof pb.UnregisterRequest
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        UnregisterRequest.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            return null;
        };

        /**
         * Creates an UnregisterRequest message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.UnregisterRequest
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.UnregisterRequest} UnregisterRequest
         */
        UnregisterRequest.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.UnregisterRequest)
                return object;
            let message = new $root.pb.UnregisterRequest();
            if (object.pid != null)
                message.pid = object.pid >>> 0;
            return message;
        };

        /**
         * Creates a plain object from an UnregisterRequest message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.UnregisterRequest
         * @static
         * @param {pb.UnregisterRequest} message UnregisterRequest
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        UnregisterRequest.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults)
                object.pid = 0;
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            return object;
        };

        /**
         * Converts this UnregisterRequest to JSON.
         * @function toJSON
         * @memberof pb.UnregisterRequest
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        UnregisterRequest.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for UnregisterRequest
         * @function getTypeUrl
         * @memberof pb.UnregisterRequest
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        UnregisterRequest.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.UnregisterRequest";
        };

        return UnregisterRequest;
    })();

    pb.UnregisterResponse = (function() {

        /**
         * Properties of an UnregisterResponse.
         * @memberof pb
         * @interface IUnregisterResponse
         * @property {boolean|null} [success] UnregisterResponse success
         * @property {string|null} [message] UnregisterResponse message
         */

        /**
         * Constructs a new UnregisterResponse.
         * @memberof pb
         * @classdesc Represents an UnregisterResponse.
         * @implements IUnregisterResponse
         * @constructor
         * @param {pb.IUnregisterResponse=} [properties] Properties to set
         */
        function UnregisterResponse(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * UnregisterResponse success.
         * @member {boolean} success
         * @memberof pb.UnregisterResponse
         * @instance
         */
        UnregisterResponse.prototype.success = false;

        /**
         * UnregisterResponse message.
         * @member {string} message
         * @memberof pb.UnregisterResponse
         * @instance
         */
        UnregisterResponse.prototype.message = "";

        /**
         * Creates a new UnregisterResponse instance using the specified properties.
         * @function create
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.IUnregisterResponse=} [properties] Properties to set
         * @returns {pb.UnregisterResponse} UnregisterResponse instance
         */
        UnregisterResponse.create = function create(properties) {
            return new UnregisterResponse(properties);
        };

        /**
         * Encodes the specified UnregisterResponse message. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @function encode
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.IUnregisterResponse} message UnregisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterResponse.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.success != null && Object.hasOwnProperty.call(message, "success"))
                writer.uint32(/* id 1, wireType 0 =*/8).bool(message.success);
            if (message.message != null && Object.hasOwnProperty.call(message, "message"))
                writer.uint32(/* id 2, wireType 2 =*/18).string(message.message);
            return writer;
        };

        /**
         * Encodes the specified UnregisterResponse message, length delimited. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.IUnregisterResponse} message UnregisterResponse message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        UnregisterResponse.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer.
         * @function decode
         * @memberof pb.UnregisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.UnregisterResponse} UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterResponse.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.UnregisterResponse();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.success = reader.bool();
                        break;
                    }
                case 2: {
                        message.message = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.UnregisterResponse
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.UnregisterResponse} UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        UnregisterResponse.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an UnregisterResponse message.
         * @function verify
         * @memberof pb.UnregisterResponse
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        UnregisterResponse.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.success != null && message.hasOwnProperty("success"))
                if (typeof message.success !== "boolean")
                    return "success: boolean expected";
            if (message.message != null && message.hasOwnProperty("message"))
                if (!$util.isString(message.message))
                    return "message: string expected";
            return null;
        };

        /**
         * Creates an UnregisterResponse message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.UnregisterResponse
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.UnregisterResponse} UnregisterResponse
         */
        UnregisterResponse.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.UnregisterResponse)
                return object;
            let message = new $root.pb.UnregisterResponse();
            if (object.success != null)
                message.success = Boolean(object.success);
            if (object.message != null)
                message.message = String(object.message);
            return message;
        };

        /**
         * Creates a plain object from an UnregisterResponse message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.UnregisterResponse
         * @static
         * @param {pb.UnregisterResponse} message UnregisterResponse
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        UnregisterResponse.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.success = false;
                object.message = "";
            }
            if (message.success != null && message.hasOwnProperty("success"))
                object.success = message.success;
            if (message.message != null && message.hasOwnProperty("message"))
                object.message = message.message;
            return object;
        };

        /**
         * Converts this UnregisterResponse to JSON.
         * @function toJSON
         * @memberof pb.UnregisterResponse
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        UnregisterResponse.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for UnregisterResponse
         * @function getTypeUrl
         * @memberof pb.UnregisterResponse
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        UnregisterResponse.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.UnregisterResponse";
        };

        return UnregisterResponse;
    })();

    pb.Event = (function() {

        /**
         * Properties of an Event.
         * @memberof pb
         * @interface IEvent
         * @property {number|null} [pid] Event pid
         * @property {number|null} [ppid] Event ppid
         * @property {number|null} [uid] Event uid
         * @property {string|null} [type] Event type
         * @property {string|null} [tag] Event tag
         * @property {string|null} [comm] Event comm
         * @property {string|null} [path] Event path
         */

        /**
         * Constructs a new Event.
         * @memberof pb
         * @classdesc Represents an Event.
         * @implements IEvent
         * @constructor
         * @param {pb.IEvent=} [properties] Properties to set
         */
        function Event(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Event pid.
         * @member {number} pid
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.pid = 0;

        /**
         * Event ppid.
         * @member {number} ppid
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.ppid = 0;

        /**
         * Event uid.
         * @member {number} uid
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.uid = 0;

        /**
         * Event type.
         * @member {string} type
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.type = "";

        /**
         * Event tag.
         * @member {string} tag
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.tag = "";

        /**
         * Event comm.
         * @member {string} comm
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.comm = "";

        /**
         * Event path.
         * @member {string} path
         * @memberof pb.Event
         * @instance
         */
        Event.prototype.path = "";

        /**
         * Creates a new Event instance using the specified properties.
         * @function create
         * @memberof pb.Event
         * @static
         * @param {pb.IEvent=} [properties] Properties to set
         * @returns {pb.Event} Event instance
         */
        Event.create = function create(properties) {
            return new Event(properties);
        };

        /**
         * Encodes the specified Event message. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @function encode
         * @memberof pb.Event
         * @static
         * @param {pb.IEvent} message Event message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Event.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).uint32(message.pid);
            if (message.ppid != null && Object.hasOwnProperty.call(message, "ppid"))
                writer.uint32(/* id 2, wireType 0 =*/16).uint32(message.ppid);
            if (message.uid != null && Object.hasOwnProperty.call(message, "uid"))
                writer.uint32(/* id 3, wireType 0 =*/24).uint32(message.uid);
            if (message.type != null && Object.hasOwnProperty.call(message, "type"))
                writer.uint32(/* id 4, wireType 2 =*/34).string(message.type);
            if (message.tag != null && Object.hasOwnProperty.call(message, "tag"))
                writer.uint32(/* id 5, wireType 2 =*/42).string(message.tag);
            if (message.comm != null && Object.hasOwnProperty.call(message, "comm"))
                writer.uint32(/* id 6, wireType 2 =*/50).string(message.comm);
            if (message.path != null && Object.hasOwnProperty.call(message, "path"))
                writer.uint32(/* id 7, wireType 2 =*/58).string(message.path);
            return writer;
        };

        /**
         * Encodes the specified Event message, length delimited. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.Event
         * @static
         * @param {pb.IEvent} message Event message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Event.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes an Event message from the specified reader or buffer.
         * @function decode
         * @memberof pb.Event
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.Event} Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Event.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.Event();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.uint32();
                        break;
                    }
                case 2: {
                        message.ppid = reader.uint32();
                        break;
                    }
                case 3: {
                        message.uid = reader.uint32();
                        break;
                    }
                case 4: {
                        message.type = reader.string();
                        break;
                    }
                case 5: {
                        message.tag = reader.string();
                        break;
                    }
                case 6: {
                        message.comm = reader.string();
                        break;
                    }
                case 7: {
                        message.path = reader.string();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes an Event message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.Event
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.Event} Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Event.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies an Event message.
         * @function verify
         * @memberof pb.Event
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Event.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                if (!$util.isInteger(message.ppid))
                    return "ppid: integer expected";
            if (message.uid != null && message.hasOwnProperty("uid"))
                if (!$util.isInteger(message.uid))
                    return "uid: integer expected";
            if (message.type != null && message.hasOwnProperty("type"))
                if (!$util.isString(message.type))
                    return "type: string expected";
            if (message.tag != null && message.hasOwnProperty("tag"))
                if (!$util.isString(message.tag))
                    return "tag: string expected";
            if (message.comm != null && message.hasOwnProperty("comm"))
                if (!$util.isString(message.comm))
                    return "comm: string expected";
            if (message.path != null && message.hasOwnProperty("path"))
                if (!$util.isString(message.path))
                    return "path: string expected";
            return null;
        };

        /**
         * Creates an Event message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.Event
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.Event} Event
         */
        Event.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.Event)
                return object;
            let message = new $root.pb.Event();
            if (object.pid != null)
                message.pid = object.pid >>> 0;
            if (object.ppid != null)
                message.ppid = object.ppid >>> 0;
            if (object.uid != null)
                message.uid = object.uid >>> 0;
            if (object.type != null)
                message.type = String(object.type);
            if (object.tag != null)
                message.tag = String(object.tag);
            if (object.comm != null)
                message.comm = String(object.comm);
            if (object.path != null)
                message.path = String(object.path);
            return message;
        };

        /**
         * Creates a plain object from an Event message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.Event
         * @static
         * @param {pb.Event} message Event
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Event.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.pid = 0;
                object.ppid = 0;
                object.uid = 0;
                object.type = "";
                object.tag = "";
                object.comm = "";
                object.path = "";
            }
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                object.ppid = message.ppid;
            if (message.uid != null && message.hasOwnProperty("uid"))
                object.uid = message.uid;
            if (message.type != null && message.hasOwnProperty("type"))
                object.type = message.type;
            if (message.tag != null && message.hasOwnProperty("tag"))
                object.tag = message.tag;
            if (message.comm != null && message.hasOwnProperty("comm"))
                object.comm = message.comm;
            if (message.path != null && message.hasOwnProperty("path"))
                object.path = message.path;
            return object;
        };

        /**
         * Converts this Event to JSON.
         * @function toJSON
         * @memberof pb.Event
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Event.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Event
         * @function getTypeUrl
         * @memberof pb.Event
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Event.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.Event";
        };

        return Event;
    })();

    pb.Process = (function() {

        /**
         * Properties of a Process.
         * @memberof pb
         * @interface IProcess
         * @property {number|null} [pid] Process pid
         * @property {number|null} [ppid] Process ppid
         * @property {string|null} [name] Process name
         * @property {number|null} [cpu] Process cpu
         * @property {number|null} [mem] Process mem
         * @property {string|null} [user] Process user
         * @property {number|null} [gpuMem] Process gpuMem
         */

        /**
         * Constructs a new Process.
         * @memberof pb
         * @classdesc Represents a Process.
         * @implements IProcess
         * @constructor
         * @param {pb.IProcess=} [properties] Properties to set
         */
        function Process(properties) {
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * Process pid.
         * @member {number} pid
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.pid = 0;

        /**
         * Process ppid.
         * @member {number} ppid
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.ppid = 0;

        /**
         * Process name.
         * @member {string} name
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.name = "";

        /**
         * Process cpu.
         * @member {number} cpu
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.cpu = 0;

        /**
         * Process mem.
         * @member {number} mem
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.mem = 0;

        /**
         * Process user.
         * @member {string} user
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.user = "";

        /**
         * Process gpuMem.
         * @member {number} gpuMem
         * @memberof pb.Process
         * @instance
         */
        Process.prototype.gpuMem = 0;

        /**
         * Creates a new Process instance using the specified properties.
         * @function create
         * @memberof pb.Process
         * @static
         * @param {pb.IProcess=} [properties] Properties to set
         * @returns {pb.Process} Process instance
         */
        Process.create = function create(properties) {
            return new Process(properties);
        };

        /**
         * Encodes the specified Process message. Does not implicitly {@link pb.Process.verify|verify} messages.
         * @function encode
         * @memberof pb.Process
         * @static
         * @param {pb.IProcess} message Process message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Process.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.pid != null && Object.hasOwnProperty.call(message, "pid"))
                writer.uint32(/* id 1, wireType 0 =*/8).int32(message.pid);
            if (message.ppid != null && Object.hasOwnProperty.call(message, "ppid"))
                writer.uint32(/* id 2, wireType 0 =*/16).int32(message.ppid);
            if (message.name != null && Object.hasOwnProperty.call(message, "name"))
                writer.uint32(/* id 3, wireType 2 =*/26).string(message.name);
            if (message.cpu != null && Object.hasOwnProperty.call(message, "cpu"))
                writer.uint32(/* id 4, wireType 1 =*/33).double(message.cpu);
            if (message.mem != null && Object.hasOwnProperty.call(message, "mem"))
                writer.uint32(/* id 5, wireType 5 =*/45).float(message.mem);
            if (message.user != null && Object.hasOwnProperty.call(message, "user"))
                writer.uint32(/* id 6, wireType 2 =*/50).string(message.user);
            if (message.gpuMem != null && Object.hasOwnProperty.call(message, "gpuMem"))
                writer.uint32(/* id 7, wireType 0 =*/56).uint32(message.gpuMem);
            return writer;
        };

        /**
         * Encodes the specified Process message, length delimited. Does not implicitly {@link pb.Process.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.Process
         * @static
         * @param {pb.IProcess} message Process message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        Process.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a Process message from the specified reader or buffer.
         * @function decode
         * @memberof pb.Process
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.Process} Process
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Process.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.Process();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        message.pid = reader.int32();
                        break;
                    }
                case 2: {
                        message.ppid = reader.int32();
                        break;
                    }
                case 3: {
                        message.name = reader.string();
                        break;
                    }
                case 4: {
                        message.cpu = reader.double();
                        break;
                    }
                case 5: {
                        message.mem = reader.float();
                        break;
                    }
                case 6: {
                        message.user = reader.string();
                        break;
                    }
                case 7: {
                        message.gpuMem = reader.uint32();
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a Process message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.Process
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.Process} Process
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        Process.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a Process message.
         * @function verify
         * @memberof pb.Process
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        Process.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.pid != null && message.hasOwnProperty("pid"))
                if (!$util.isInteger(message.pid))
                    return "pid: integer expected";
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                if (!$util.isInteger(message.ppid))
                    return "ppid: integer expected";
            if (message.name != null && message.hasOwnProperty("name"))
                if (!$util.isString(message.name))
                    return "name: string expected";
            if (message.cpu != null && message.hasOwnProperty("cpu"))
                if (typeof message.cpu !== "number")
                    return "cpu: number expected";
            if (message.mem != null && message.hasOwnProperty("mem"))
                if (typeof message.mem !== "number")
                    return "mem: number expected";
            if (message.user != null && message.hasOwnProperty("user"))
                if (!$util.isString(message.user))
                    return "user: string expected";
            if (message.gpuMem != null && message.hasOwnProperty("gpuMem"))
                if (!$util.isInteger(message.gpuMem))
                    return "gpuMem: integer expected";
            return null;
        };

        /**
         * Creates a Process message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.Process
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.Process} Process
         */
        Process.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.Process)
                return object;
            let message = new $root.pb.Process();
            if (object.pid != null)
                message.pid = object.pid | 0;
            if (object.ppid != null)
                message.ppid = object.ppid | 0;
            if (object.name != null)
                message.name = String(object.name);
            if (object.cpu != null)
                message.cpu = Number(object.cpu);
            if (object.mem != null)
                message.mem = Number(object.mem);
            if (object.user != null)
                message.user = String(object.user);
            if (object.gpuMem != null)
                message.gpuMem = object.gpuMem >>> 0;
            return message;
        };

        /**
         * Creates a plain object from a Process message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.Process
         * @static
         * @param {pb.Process} message Process
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        Process.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.defaults) {
                object.pid = 0;
                object.ppid = 0;
                object.name = "";
                object.cpu = 0;
                object.mem = 0;
                object.user = "";
                object.gpuMem = 0;
            }
            if (message.pid != null && message.hasOwnProperty("pid"))
                object.pid = message.pid;
            if (message.ppid != null && message.hasOwnProperty("ppid"))
                object.ppid = message.ppid;
            if (message.name != null && message.hasOwnProperty("name"))
                object.name = message.name;
            if (message.cpu != null && message.hasOwnProperty("cpu"))
                object.cpu = options.json && !isFinite(message.cpu) ? String(message.cpu) : message.cpu;
            if (message.mem != null && message.hasOwnProperty("mem"))
                object.mem = options.json && !isFinite(message.mem) ? String(message.mem) : message.mem;
            if (message.user != null && message.hasOwnProperty("user"))
                object.user = message.user;
            if (message.gpuMem != null && message.hasOwnProperty("gpuMem"))
                object.gpuMem = message.gpuMem;
            return object;
        };

        /**
         * Converts this Process to JSON.
         * @function toJSON
         * @memberof pb.Process
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        Process.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for Process
         * @function getTypeUrl
         * @memberof pb.Process
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        Process.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.Process";
        };

        return Process;
    })();

    pb.ProcessList = (function() {

        /**
         * Properties of a ProcessList.
         * @memberof pb
         * @interface IProcessList
         * @property {Array.<pb.IProcess>|null} [processes] ProcessList processes
         */

        /**
         * Constructs a new ProcessList.
         * @memberof pb
         * @classdesc Represents a ProcessList.
         * @implements IProcessList
         * @constructor
         * @param {pb.IProcessList=} [properties] Properties to set
         */
        function ProcessList(properties) {
            this.processes = [];
            if (properties)
                for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                    if (properties[keys[i]] != null)
                        this[keys[i]] = properties[keys[i]];
        }

        /**
         * ProcessList processes.
         * @member {Array.<pb.IProcess>} processes
         * @memberof pb.ProcessList
         * @instance
         */
        ProcessList.prototype.processes = $util.emptyArray;

        /**
         * Creates a new ProcessList instance using the specified properties.
         * @function create
         * @memberof pb.ProcessList
         * @static
         * @param {pb.IProcessList=} [properties] Properties to set
         * @returns {pb.ProcessList} ProcessList instance
         */
        ProcessList.create = function create(properties) {
            return new ProcessList(properties);
        };

        /**
         * Encodes the specified ProcessList message. Does not implicitly {@link pb.ProcessList.verify|verify} messages.
         * @function encode
         * @memberof pb.ProcessList
         * @static
         * @param {pb.IProcessList} message ProcessList message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ProcessList.encode = function encode(message, writer) {
            if (!writer)
                writer = $Writer.create();
            if (message.processes != null && message.processes.length)
                for (let i = 0; i < message.processes.length; ++i)
                    $root.pb.Process.encode(message.processes[i], writer.uint32(/* id 1, wireType 2 =*/10).fork()).ldelim();
            return writer;
        };

        /**
         * Encodes the specified ProcessList message, length delimited. Does not implicitly {@link pb.ProcessList.verify|verify} messages.
         * @function encodeDelimited
         * @memberof pb.ProcessList
         * @static
         * @param {pb.IProcessList} message ProcessList message or plain object to encode
         * @param {$protobuf.Writer} [writer] Writer to encode to
         * @returns {$protobuf.Writer} Writer
         */
        ProcessList.encodeDelimited = function encodeDelimited(message, writer) {
            return this.encode(message, writer).ldelim();
        };

        /**
         * Decodes a ProcessList message from the specified reader or buffer.
         * @function decode
         * @memberof pb.ProcessList
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @param {number} [length] Message length if known beforehand
         * @returns {pb.ProcessList} ProcessList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ProcessList.decode = function decode(reader, length, error) {
            if (!(reader instanceof $Reader))
                reader = $Reader.create(reader);
            let end = length === undefined ? reader.len : reader.pos + length, message = new $root.pb.ProcessList();
            while (reader.pos < end) {
                let tag = reader.uint32();
                if (tag === error)
                    break;
                switch (tag >>> 3) {
                case 1: {
                        if (!(message.processes && message.processes.length))
                            message.processes = [];
                        message.processes.push($root.pb.Process.decode(reader, reader.uint32()));
                        break;
                    }
                default:
                    reader.skipType(tag & 7);
                    break;
                }
            }
            return message;
        };

        /**
         * Decodes a ProcessList message from the specified reader or buffer, length delimited.
         * @function decodeDelimited
         * @memberof pb.ProcessList
         * @static
         * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
         * @returns {pb.ProcessList} ProcessList
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        ProcessList.decodeDelimited = function decodeDelimited(reader) {
            if (!(reader instanceof $Reader))
                reader = new $Reader(reader);
            return this.decode(reader, reader.uint32());
        };

        /**
         * Verifies a ProcessList message.
         * @function verify
         * @memberof pb.ProcessList
         * @static
         * @param {Object.<string,*>} message Plain object to verify
         * @returns {string|null} `null` if valid, otherwise the reason why it is not
         */
        ProcessList.verify = function verify(message) {
            if (typeof message !== "object" || message === null)
                return "object expected";
            if (message.processes != null && message.hasOwnProperty("processes")) {
                if (!Array.isArray(message.processes))
                    return "processes: array expected";
                for (let i = 0; i < message.processes.length; ++i) {
                    let error = $root.pb.Process.verify(message.processes[i]);
                    if (error)
                        return "processes." + error;
                }
            }
            return null;
        };

        /**
         * Creates a ProcessList message from a plain object. Also converts values to their respective internal types.
         * @function fromObject
         * @memberof pb.ProcessList
         * @static
         * @param {Object.<string,*>} object Plain object
         * @returns {pb.ProcessList} ProcessList
         */
        ProcessList.fromObject = function fromObject(object) {
            if (object instanceof $root.pb.ProcessList)
                return object;
            let message = new $root.pb.ProcessList();
            if (object.processes) {
                if (!Array.isArray(object.processes))
                    throw TypeError(".pb.ProcessList.processes: array expected");
                message.processes = [];
                for (let i = 0; i < object.processes.length; ++i) {
                    if (typeof object.processes[i] !== "object")
                        throw TypeError(".pb.ProcessList.processes: object expected");
                    message.processes[i] = $root.pb.Process.fromObject(object.processes[i]);
                }
            }
            return message;
        };

        /**
         * Creates a plain object from a ProcessList message. Also converts values to other types if specified.
         * @function toObject
         * @memberof pb.ProcessList
         * @static
         * @param {pb.ProcessList} message ProcessList
         * @param {$protobuf.IConversionOptions} [options] Conversion options
         * @returns {Object.<string,*>} Plain object
         */
        ProcessList.toObject = function toObject(message, options) {
            if (!options)
                options = {};
            let object = {};
            if (options.arrays || options.defaults)
                object.processes = [];
            if (message.processes && message.processes.length) {
                object.processes = [];
                for (let j = 0; j < message.processes.length; ++j)
                    object.processes[j] = $root.pb.Process.toObject(message.processes[j], options);
            }
            return object;
        };

        /**
         * Converts this ProcessList to JSON.
         * @function toJSON
         * @memberof pb.ProcessList
         * @instance
         * @returns {Object.<string,*>} JSON object
         */
        ProcessList.prototype.toJSON = function toJSON() {
            return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
        };

        /**
         * Gets the default type url for ProcessList
         * @function getTypeUrl
         * @memberof pb.ProcessList
         * @static
         * @param {string} [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns {string} The default type url
         */
        ProcessList.getTypeUrl = function getTypeUrl(typeUrlPrefix) {
            if (typeUrlPrefix === undefined) {
                typeUrlPrefix = "type.googleapis.com";
            }
            return typeUrlPrefix + "/pb.ProcessList";
        };

        return ProcessList;
    })();

    return pb;
})();

export { $root as default };
